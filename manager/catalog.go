package manager

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/rancher/go-rancher/client"
	"github.com/rancher/rancher-catalog-service/model"
	"gopkg.in/yaml.v2"
)

var (
	metadataFolder = regexp.MustCompile(`^DATA/[^/]+/templates/[^/]+$`)
)

//CatalogCollection holds a collection of catalogs
type CatalogCollection struct {
	client.Collection
	Data []Catalog `json:"data,omitempty"`
}

//Catalog defines the properties of a template Catalog
type Catalog struct {
	client.Resource
	CatalogID         string `json:"id"`
	Description       string `json:"description"`
	CatalogLink       string `json:"catalogLink"`
	url               string
	catalogRoot       string
	refreshReqChannel *chan int
	metadata          map[string]model.Template
}

func (cat *Catalog) getID() string {
	return cat.CatalogID
}

func (cat *Catalog) cloneCatalog() {
	log.Infof("Cloning the catalog from git url %s", cat.url)
	//git clone the repo
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
		newTemplate := model.Template{
			Resource: client.Resource{
				Id:   cat.CatalogID + ":" + f.Name(),
				Type: "template",
			},
			Path: cat.CatalogID + "/" + f.Name(), //catalogRoot + f.Name()
		}

		//read the root level config.yml
		readTemplateConfig(path, &newTemplate)

		//list the folders under the root level
		newTemplate.VersionLinks = make(map[string]string)
		newTemplate.TemplateVersionRancherVersion = make(map[string]string)
		dirList, err := ioutil.ReadDir(path)
		if err != nil {
			log.Errorf("Error reading directories at path: %s, error: %v", f.Name(), err)
		} else {
			for _, subfile := range dirList {
				if subfile.IsDir() {
					//read the subversion config.yml file into a template
					subTemplate := model.Template{}
					err := readRancherCompose(path+"/"+subfile.Name(), &subTemplate)
					if err == nil {
						newTemplate.VersionLinks[subTemplate.Version] = newTemplate.Id + ":" + subfile.Name()
						newTemplate.TemplateVersionRancherVersion[subTemplate.Version] = subTemplate.MinimumRancherVersion
					} else {
						log.Errorf("Skipping the template version: %s, error: %v", f.Name()+"/"+subfile.Name(), err)
					}
				} else if strings.HasPrefix(subfile.Name(), "catalogIcon") {
					newTemplate.IconLink = newTemplate.Id + "?image"
					PathToImage[newTemplate.Path] = subfile.Name()
				} else if strings.HasPrefix(strings.ToLower(subfile.Name()), "readme") {
					newTemplate.ReadmeLink = newTemplate.Id + "?readme"
					PathToReadme[newTemplate.Path] = subfile.Name()
				}
			}
		}

		cat.metadata[newTemplate.Path] = newTemplate
	}

	return nil
}

func (cat *Catalog) pullCatalog() error {
	log.Infof("Pulling the catalog %s from the repo to sync any new changes to %s", cat.CatalogID, cat.catalogRoot)

	e := exec.Command("git", "-C", cat.catalogRoot, "pull", "-r", "origin", "master")

	err := e.Run()
	if err != nil {
		log.Errorf("Failed to pull the catalog from github repo %s, error: %v", cat.url, err.Error())
		return err
	}
	return nil
}

func (cat *Catalog) refreshCatalog() {
	//put msg on channel, so that any other request can wait
	select {
	case *cat.refreshReqChannel <- 1:
		err := cat.pullCatalog()
		if err == nil {
			log.Debugf("Refreshing the catalog %s ...", cat.getID())
			//walk the catalog and read the metadata to the cache
			cat.metadata = make(map[string]model.Template)
			filepath.Walk(cat.catalogRoot, cat.walkCatalog)
		} else {
			log.Debugf("Will not refresh the catalog since Pull Catalog faced error: %v", err)
		}
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
			template.Maintainer = config["maintainer"]
			template.License = config["license"]
			template.ProjectURL = config["projectURL"]
		}
	}
}

func readRancherCompose(relativePath string, newTemplate *model.Template) error {

	composeBytes := readFile(relativePath, "rancher-compose.yml")
	newTemplate.RancherCompose = string(*composeBytes)

	//read the questions section
	RC := make(map[string]model.RancherCompose)
	err := yaml.Unmarshal(*composeBytes, &RC)
	if err != nil {
		log.Errorf("Error unmarshalling %s under template: %s, error: %v", "rancher-compose.yml", relativePath, err)
		return err
	}
	newTemplate.Questions = RC[".catalog"].Questions
	newTemplate.Name = RC[".catalog"].Name
	newTemplate.UUID = RC[".catalog"].UUID
	newTemplate.Description = RC[".catalog"].Description
	newTemplate.Version = RC[".catalog"].Version
	newTemplate.MinimumRancherVersion = RC[".catalog"].MinimumRancherVersion
	newTemplate.Output = RC[".catalog"].Output

	return nil
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
func (cat *Catalog) ReadTemplateVersion(templateID string, versionID string) (*model.Template, bool) {

	path := cat.CatalogID + "/templates/" + templateID + "/" + versionID
	parentPath := cat.CatalogID + "/" + templateID
	_, ok := cat.metadata[parentPath]

	if ok {
		dirList, err := ioutil.ReadDir(CatalogRootDir + path)
		newTemplate := model.Template{}
		newTemplate.Path = cat.CatalogID + "/" + templateID + "/" + versionID
		newTemplate.Id = cat.CatalogID + ":" + templateID + ":" + versionID

		if err != nil {
			log.Errorf("Error reading template at path: %s, error: %v", path, err)
			return nil, false
		}

		var foundIcon, foundReadme bool

		for _, subfile := range dirList {
			if strings.HasPrefix(subfile.Name(), "catalogIcon") {

				newTemplate.IconLink = newTemplate.Id + "?image"
				foundIcon = true
				PathToImage[newTemplate.Path] = subfile.Name()

			} else if strings.HasPrefix(subfile.Name(), "docker-compose") {

				newTemplate.DockerCompose = string(*(readFile(CatalogRootDir+path, subfile.Name())))

			} else if strings.HasPrefix(subfile.Name(), "rancher-compose") {

				readRancherCompose(CatalogRootDir+path, &newTemplate)
			} else if strings.HasPrefix(strings.ToLower(subfile.Name()), "readme") {

				newTemplate.ReadmeLink = newTemplate.Id + "?readme"
				foundReadme = true
				PathToReadme[newTemplate.Path] = subfile.Name()

			}
		}
		if !foundIcon {
			//use the parent icon
			parentMetadata, ok := cat.metadata[parentPath]
			if ok {
				newTemplate.IconLink = parentMetadata.IconLink
			} else {
				log.Debugf("Could not find the parent metadata %s", parentPath)
			}
		}

		if !foundReadme {
			//use the parent readme
			parentMetadata, ok := cat.metadata[parentPath]
			if ok {
				newTemplate.ReadmeLink = parentMetadata.ReadmeLink
			} else {
				log.Debugf("Could not find the parent metadata %s", parentPath)
			}
		}

		return &newTemplate, true
	}

	return nil, false

}
