package manager

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	log "github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-catalog-service/model"
)

var (
	catalogURL        = flag.String("catalogUrl", "", "GitHub public repo url containing catalog")
	refreshInterval   = flag.Int64("refreshInterval", 60, "Time interval (in Seconds) to periodically pull the catalog from github repo")
	logFile           = flag.String("logFile", "", "Log file")
	debug             = flag.Bool("debug", false, "Debug")
	metadataFolder    = regexp.MustCompile(`^DATA/templates/[^/]+$`)
	refreshReqChannel = make(chan int, 1)
	//Catalog is the map storing template metadata in memory
	Catalog map[string]model.Template
	//UUIDToPath holds the mapping between a template UUID to the path in the repo
	UUIDToPath map[string]string
	//CatalogReadyChannel signals if the catalog is cloned and loaded in memmory
	CatalogReadyChannel = make(chan int, 1)
)

const catalogRoot string = "./DATA/templates/"

//SetEnv parses the command line args and sets the necessary variables
func SetEnv() {
	flag.Parse()

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	if *logFile != "" {
		if output, err := os.OpenFile(*logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666); err != nil {
			log.Fatalf("Failed to log to file %s: %v", *logFile, err)
		} else {
			log.SetOutput(output)
		}
	}

	textFormatter := &log.TextFormatter{
		FullTimestamp: true,
	}
	log.SetFormatter(textFormatter)

	if *catalogURL == "" {
		err := "Halting Catalog service, Catalog github repo url not provided"
		log.Fatal(err)
		_ = fmt.Errorf(err)
	}
}

//Init clones or pulls the catalog, starts background refresh thread
func Init() {
	_, err := os.Stat(catalogRoot)
	if err == nil {
		//remove the existing repo
		err := os.RemoveAll("./DATA/")
		if err != nil {
			log.Fatal("Cannot remove the existing catalog data folder ./DATA/", err)
			_ = fmt.Errorf("Cannot remove the existing catalog data folder ./DATA/, error: " + err.Error())
		}
	}
	cloneCatalog()
	UUIDToPath = make(map[string]string)
	Catalog = make(map[string]model.Template)
	filepath.Walk(catalogRoot, walkCatalog)

	//start a background timer to pull from the Catalog periodically
	startCatalogBackgroundPoll()
	CatalogReadyChannel <- 1
}

func startCatalogBackgroundPoll() {
	ticker := time.NewTicker(time.Duration(*refreshInterval) * time.Second)
	go func() {
		for t := range ticker.C {
			log.Debugf("Running background Catalog Refresh Thread at time %s", t)
			RefreshCatalog()
		}
	}()
}

//RefreshCatalog syncs the catalog from the repo
func RefreshCatalog() {
	//put msg on channel, so that any other request can wait
	select {
	case refreshReqChannel <- 1:
		pullCatalog()
		filepath.Walk(catalogRoot, walkCatalog)
		<-refreshReqChannel
	default:
		log.Info("Refresh catalog is already in process, skipping")
	}
}

func cloneCatalog() {
	log.Infof("Cloning the catalog from github url %s", *catalogURL)
	//git clone the github repo
	e := exec.Command("git", "clone", *catalogURL, "./DATA")
	e.Stdout = os.Stdout
	e.Stderr = os.Stderr
	err := e.Run()
	if err != nil {
		log.Fatal("Failed to clone the catalog from github", err.Error())
	}
}

func pullCatalog() {
	log.Debug("Pulling the catalog from the repo to sync any new changes")

	e := exec.Command("git", "-C", "./DATA", "pull", "origin", "master")
	err := e.Run()
	if err != nil {
		log.Errorf("Failed to pull the catalog from github repo %s, error: %v", *catalogURL, err)
	}
}

func walkCatalog(path string, f os.FileInfo, err error) error {

	if f.IsDir() && metadataFolder.MatchString(path) {

		log.Debugf("Reading metadata folder for template:%s", f.Name())
		newTemplate := model.Template{}
		newTemplate.Path = f.Name()

		//read the root level config.yml
		readTemplateConfig(path, &newTemplate)

		//list the folders under the root level
		newTemplate.VersionLinks = make(map[string]string)
		dirList, err := ioutil.ReadDir(path)
		if err != nil {
			log.Errorf("Error reading directories at path: %s, error: %v", f.Name(), err)
		} else {
			for _, subfile := range dirList {
				if subfile.IsDir() {
					//read the subversion config.yml file into a template
					subTemplate := model.Template{}
					readRancherCompose(f.Name()+"/"+subfile.Name(), &subTemplate)
					if subTemplate.UUID != "" {
						UUIDToPath[subTemplate.UUID] = f.Name() + "/" + subfile.Name()
						log.Debugf("UUIDToPath map: %v", UUIDToPath)
					}
					newTemplate.VersionLinks[subTemplate.Version] = f.Name() + "/" + subfile.Name()
				} else if strings.HasPrefix(subfile.Name(), "catalogIcon") {
					newTemplate.IconLink = f.Name() + "/" + subfile.Name()
				}
			}
		}

		Catalog[f.Name()] = newTemplate
	}
	return nil
}

//ReadTemplateVersion reads the template version details
func ReadTemplateVersion(path string) model.Template {

	dirList, err := ioutil.ReadDir(catalogRoot + path)
	newTemplate := model.Template{}
	newTemplate.Path = path

	if err != nil {
		log.Errorf("Error reading template at path: %s, error: %v", path, err)
	} else {

		var foundIcon bool

		for _, subfile := range dirList {
			if strings.HasPrefix(subfile.Name(), "catalogIcon") {

				newTemplate.IconLink = path + "/" + subfile.Name()
				foundIcon = true

			} else if strings.HasPrefix(subfile.Name(), "docker-compose") {

				newTemplate.DockerCompose = string(*(readFile(catalogRoot+path, subfile.Name())))

			} else if strings.HasPrefix(subfile.Name(), "rancher-compose") {

				readRancherCompose(path, &newTemplate)
			}
		}
		if !foundIcon {
			//use the parent icon
			tokens := strings.Split(path, "/")
			parentPath := tokens[0]
			parentMetadata, ok := Catalog[parentPath]
			if ok {
				newTemplate.IconLink = parentMetadata.IconLink
			} else {
				log.Debugf("Could not find the parent metadata %s", parentPath)
			}
		}
	}

	return newTemplate

}

func readTemplateConfig(relativePath string, template *model.Template) {
	filename, err := filepath.Abs(relativePath + "/config.yml")
	if err != nil {
		log.Errorf("Error forming path to config file at path: %s, error: %v", relativePath, err)
	}

	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Errorf("Error reading config file under template: %s, error: %v", relativePath, err)
	} else {
		config := make(map[string]string)

		//Read the config.yml file
		err = yaml.Unmarshal(yamlFile, &config)
		if err != nil {
			log.Errorf("Error unmarshalling config.yml under template: %s, error: %v", relativePath, err)
		} else {
			template.Name = config["name"]
			template.Category = config["category"]
			template.Description = config["description"]
			template.Version = config["version"]
			if config["uuid"] != "" {
				template.UUID = config["uuid"]
			}
		}
	}
}

func readRancherCompose(relativePath string, newTemplate *model.Template) {

	composeBytes := readFile(catalogRoot+relativePath, "rancher-compose.yml")
	newTemplate.RancherCompose = string(*composeBytes)

	//read the questions section
	RC := make(map[string]model.RancherCompose)
	err := yaml.Unmarshal(*composeBytes, &RC)
	if err != nil {
		log.Errorf("Error unmarshalling %s under template: %s, error: %v", "rancher-compose.yml", relativePath, err)
	} else {
		newTemplate.Questions = RC[".catalog"].Questions
		newTemplate.Name = RC[".catalog"].Name
		newTemplate.UUID = RC[".catalog"].UUID
		newTemplate.Description = RC[".catalog"].Description
		newTemplate.Version = RC[".catalog"].Version

		if newTemplate.UUID != "" {
			//store uuid -> path map
			UUIDToPath[newTemplate.UUID] = relativePath
			log.Debugf("UUIDToPath map: %v", UUIDToPath)
		}
	}

}

func readFile(relativePath string, fileName string) *[]byte {
	filename, err := filepath.Abs(relativePath + "/" + fileName)
	if err != nil {
		log.Errorf("Error forming path to file %s, error: %v", relativePath+"/"+fileName, err)
	}

	composeBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Errorf("Error reading file %s, error: %v", relativePath+"/"+fileName, err)
		return nil
	}
	return &composeBytes
}

//GetNewTemplateVersions gets new versions of a template if available
func GetNewTemplateVersions(templateUUID string) (model.Template, bool) {
	templateMetadata := model.Template{}
	path := UUIDToPath[templateUUID]
	if path != "" {
		//refresh the catalog and sync any new changes
		RefreshCatalog()

		//find the base template metadata name
		tokens := strings.Split(path, "/")
		parentPath := tokens[0]
		cVersion := tokens[1]
		currentVersion, err := strconv.Atoi(cVersion)

		if err != nil {
			log.Debugf("Error %v reading Current Version from path: %s for uuid: %s", err, path, templateUUID)
		} else {
			templateMetadata, ok := Catalog[parentPath]
			if ok {
				log.Debugf("Template found by uuid: %s", templateUUID)
				copyOfversionLinks := make(map[string]string)
				for key, value := range templateMetadata.VersionLinks {
					if value != path {
						otherVersionTokens := strings.Split(value, "/")
						oVersion := otherVersionTokens[1]
						otherVersion, err := strconv.Atoi(oVersion)

						if err == nil && otherVersion > currentVersion {
							copyOfversionLinks[key] = value
						}
					} else {
						templateMetadata.Version = key
					}
				}
				templateMetadata.VersionLinks = copyOfversionLinks
				return templateMetadata, true
			}
		}
	} else {
		log.Debugf("Template  path not found by uuid: %s", templateUUID)
	}

	log.Debugf("Template metadata not found by uuid: %s", templateUUID)
	return templateMetadata, false
}
