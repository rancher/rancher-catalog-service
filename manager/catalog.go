package manager

import (
	log "github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-catalog-service/model"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	metadataFolder = regexp.MustCompile(`^DATA/[^/]+/templates/[^/]+$`)
)

//Catalog defines the properties of a template Catalog
type Catalog struct {
	ID                string
	Description       string
	url               string
	catalogRoot       string
	refreshReqChannel *chan int
	metadata          map[string]model.Template
}

func (cat *Catalog) getID() string {
	return cat.ID
}

func (cat *Catalog) cloneCatalog() {
	log.Infof("Cloning the catalog from github url %s", cat.url)
	//git clone the github repo
	e := exec.Command("git", "clone", cat.url, cat.catalogRoot)
	e.Stdout = os.Stdout
	e.Stderr = os.Stderr
	err := e.Run()
	if err != nil {
		log.Fatalf("Failed to clone the catalog from github %v", err.Error())
	}
	cat.metadata = make(map[string]model.Template)

	//walk the catalog and read the metadata to the cache
	filepath.Walk(cat.catalogRoot, cat.walkCatalog)
}

func (cat *Catalog) walkCatalog(path string, f os.FileInfo, err error) error {
	//log.Debugf("Reading folder for template:%s %v", path, f)

	if f != nil && f.IsDir() && metadataFolder.MatchString(path) {

		log.Debugf("Reading metadata folder for template:%s, path: %v", f.Name(), path)
		newTemplate := model.Template{}
		newTemplate.Path = cat.ID + "/" + f.Name() //catalogRoot + f.Name()

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
					readRancherCompose(path+"/"+subfile.Name(), &subTemplate)
					if subTemplate.UUID != "" {
						UUIDToPath[subTemplate.UUID] = newTemplate.Path + "/" + subfile.Name()
						log.Debugf("UUIDToPath map: %v", UUIDToPath)
					}
					newTemplate.VersionLinks[subTemplate.Version] = newTemplate.Path + "/" + subfile.Name()

				} else if strings.HasPrefix(subfile.Name(), "catalogIcon") {
					newTemplate.IconLink = newTemplate.Path + "/" + subfile.Name()
				}
			}
		}

		cat.metadata[newTemplate.Path] = newTemplate
	}

	return nil
}

func (cat *Catalog) pullCatalog() {
	log.Infof("Pulling the catalog %s from github to sync any new changes to %s", cat.ID, cat.catalogRoot)

	e := exec.Command("git", "-C", cat.catalogRoot, "pull", "origin", "master")
	err := e.Run()
	if err != nil {
		log.Errorf("Failed to pull the catalog from github repo %s, error: %v", cat.url, err.Error())
	}
}

func (cat *Catalog) refreshCatalog() {
	//put msg on channel, so that any other request can wait
	select {
	case *cat.refreshReqChannel <- 1:
		cat.pullCatalog()
		//walk the catalog and read the metadata to the cache
		filepath.Walk(cat.catalogRoot, cat.walkCatalog)
		<-*cat.refreshReqChannel
	default:
		log.Infof("Refresh for this catalog %s is already in process, skipping", cat.getID())
	}
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
	composeBytes := readFile(relativePath, "rancher-compose.yml")
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

//ReadTemplateVersion reads the template version details
func (cat *Catalog) ReadTemplateVersion(templateID string, versionID string) *model.Template {

	path := cat.ID + "/templates/" + templateID + "/" + versionID
	dirList, err := ioutil.ReadDir(CatalogRootDir + path)
	newTemplate := model.Template{}
	newTemplate.Path = cat.ID + "/" + templateID + "/" + versionID

	if err != nil {
		log.Errorf("Error reading template at path: %s, error: %v", path, err)
	} else {

		var foundIcon bool

		for _, subfile := range dirList {
			if strings.HasPrefix(subfile.Name(), "catalogIcon") {

				newTemplate.IconLink = path + "/" + subfile.Name()
				foundIcon = true

			} else if strings.HasPrefix(subfile.Name(), "docker-compose") {

				newTemplate.DockerCompose = string(*(readFile(CatalogRootDir+path, subfile.Name())))

			} else if strings.HasPrefix(subfile.Name(), "rancher-compose") {

				readRancherCompose(CatalogRootDir+path, &newTemplate)
			}
		}
		if !foundIcon {
			//use the parent icon
			parentPath := cat.ID + "/" + templateID
			parentMetadata, ok := cat.metadata[parentPath]
			if ok {
				newTemplate.IconLink = parentMetadata.IconLink
			} else {
				log.Debugf("Could not find the parent metadata %s", parentPath)
			}
		}
	}

	return &newTemplate

}
