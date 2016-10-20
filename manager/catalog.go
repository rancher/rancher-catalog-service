package manager

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/rancher/go-rancher/client"
	"github.com/rancher/rancher-catalog-service/model"
	"gopkg.in/yaml.v2"
)

var (
	metadataFolder = regexp.MustCompile(`^DATA/[^/]+/((\w+)+-templates|templates)/[^/]+$`)
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
	URL               string `json:"uri"`
	State             string `json:"state"`
	LastUpdated       string `json:"lastUpdated"`
	Message           string `json:"message"`
	catalogRoot       string
	refreshReqChannel *chan int
	metadata          map[string]model.Template
	URLBranch         string `json:"branch"`
}

func (cat *Catalog) getID() string {
	return cat.CatalogID
}

func (cat *Catalog) readCatalog() error {
	_, err := os.Stat(CatalogRootDir + cat.CatalogID)
	if !os.IsNotExist(err) || err == nil {
		//catalog exists, check if url matches
		e := exec.Command("git", "-C", cat.catalogRoot, "config", "--get", "remote.origin.url")
		out, err := e.Output()
		if err != nil {
			log.Errorf("Cannot verify Git repo for Catalog %v, error: %v ", cat.CatalogID, err)
			return err
		}
		repoURL := string(out)
		repoURL = strings.TrimSpace(repoURL)
		if repoURL == cat.URL {
			log.Debugf("Catalog %v already exists with same repo url, pulling updates", cat.CatalogID)
			cat.metadata = make(map[string]model.Template)
			//walk the catalog and read the metadata to the cache
			filepath.Walk(cat.catalogRoot, cat.walkCatalog)
			if ValidationMode {
				log.Infof("Catalog loaded without errors")
				os.Exit(0)
			}
			err := cat.pullCatalog()
			if err != nil {
				log.Errorf("Git pull for Catalog %v failing with error: %v ", cat.CatalogID, err)
			}
		} else {
			//remove the existing repo
			err := os.RemoveAll(CatalogRootDir + cat.CatalogID)
			if err != nil {
				log.Errorf("Cannot remove the existing catalog folder %v, error: %v", cat.CatalogID, err)
				return err
			}
			return cat.cloneCatalog()
		}
	} else {
		log.Debugf("Catalog %v does not exist, proceeding to clone the repo : ", cat.CatalogID)
		return cat.cloneCatalog()
	}
	return nil
}

func (cat *Catalog) cloneCatalog() error {
	var e *exec.Cmd
	//git clone the repo
	// git clone -b mybranch --single-branch git://sub.domain.com/repo.git

	if cat.URLBranch == "master" {
		log.Infof("Cloning the catalog from git URL %s", cat.URL)
		e = exec.Command("git", "clone", "--recursive", cat.URL, cat.catalogRoot)
	} else {
		log.Infof("Branch : %s", cat.URLBranch)
		log.Infof("Cloning the catalog from git URL branch %s to directory %s", cat.URLBranch, cat.catalogRoot)
		e = exec.Command("git", "clone", "--recursive", "-b", cat.URLBranch, cat.URL, cat.catalogRoot)
	}

	e.Stdout = os.Stdout
	e.Stderr = os.Stderr
	err := e.Run()
	if err != nil {
		errorStr := "Failed to clone the catalog from git err: " + err.Error()
		log.Error(errorStr)
		cat.State = "error"
		cat.Message = errorStr
		return err
	}

	log.Info("Cloning completed")

	cat.metadata = make(map[string]model.Template)
	//walk the catalog and read the metadata to the cache
	filepath.Walk(cat.catalogRoot, cat.walkCatalog)
	if ValidationMode {
		log.Infof("Catalog loaded without errors")
		os.Exit(0)
	}
	cat.LastUpdated = time.Now().Format(time.RFC3339)
	cat.State = "active"
	return nil
}

func (cat *Catalog) walkCatalog(filePath string, f os.FileInfo, err error) error {
	//log.Debugf("Reading folder for template:%s %v", filePath, f)

	if f != nil && f.IsDir() && metadataFolder.MatchString(filePath) {

		//matches ./DATA/catalogID/templates/ElasticSearch or 	./DATA/catalogID/k8s-templates/ElasticSearch
		// get the prefix like 'k8s' if any
		prefix := metadataFolder.ReplaceAllString(filePath, "$2")
		prefixWithSeparator := prefix
		if prefix != "" {
			prefixWithSeparator = prefix + "*"
		}

		log.Debugf("Reading metadata folder for template:%s, path: %v", f.Name(), filePath)
		newTemplate := model.Template{
			Resource: client.Resource{
				Id:   cat.CatalogID + ":" + prefixWithSeparator + f.Name(),
				Type: "template",
			},
			Path:         cat.CatalogID + "/" + prefixWithSeparator + f.Name(), //catalogRoot + prefix + f.Name()
			TemplateBase: prefix,
		}

		//read the root level config.yml
		readTemplateConfig(filePath, &newTemplate)

		//list the folders under the root level
		newTemplate.VersionLinks = make(map[string]string)
		newTemplate.CatalogID = cat.CatalogID
		newTemplate.TemplateVersionRancherVersion = make(map[string]string)
		newTemplate.TemplateVersionRancherVersionGte = make(map[string]string)
		dirList, err := ioutil.ReadDir(filePath)
		if err != nil {
			log.Errorf("Error reading directories at path: %s, error: %v", f.Name(), err)
		} else {
			for _, subfile := range dirList {
				if subfile.IsDir() {
					//read the subversion config.yml file into a template
					subTemplate := model.Template{}
					err := readRancherCompose(path.Join(filePath, subfile.Name()), &subTemplate)
					if err == nil {
						newTemplate.VersionLinks[subTemplate.Version] = newTemplate.Id + ":" + subfile.Name()
						newTemplate.TemplateVersionRancherVersion[subTemplate.Version] = subTemplate.MinimumRancherVersion
						newTemplate.TemplateVersionRancherVersionGte[subTemplate.Version] = subTemplate.MaximumRancherVersion
					} else {
						subfilePath := path.Join(f.Name(), subfile.Name())
						if ValidationMode {
							log.Fatalf("Error processing the template version: %s, error: %v", subfilePath, err)
						}
						log.Debugf("Skipping the template version: %s, error: %v", subfilePath, err)
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
	log.Debugf("Pulling the catalog %s from the repo to sync any new changes to %s", cat.CatalogID, cat.catalogRoot)

	// git --git-dir=./DATA/value/.git/ --work-tree=./DATA/value/ checkout new_branch
	gitCheckoutCmd := exec.Command("git", "--git-dir="+cat.catalogRoot+"/.git", "--work-tree="+cat.catalogRoot, "checkout", cat.URLBranch)

	out, gitCheckoutErr := gitCheckoutCmd.Output()
	if gitCheckoutErr != nil {
		errorStr := "Git checkout failure from git err: " + gitCheckoutErr.Error()
		log.Error(errorStr)
	}
	log.Debugf("Branch to be worked on : %s\n", out)

	var e *exec.Cmd
	e = exec.Command("git", "-C", cat.catalogRoot, "pull", "-r", "origin", cat.URLBranch)

	err := e.Run()
	if err != nil {
		log.Errorf("Failed to pull the catalog from git repo %s, error: %v", cat.URL, err.Error())
		return err
	}

	log.Debugf("Update submodules of the catalog %s from the repo to sync any new changes to %s", cat.CatalogID, cat.catalogRoot)

	e = exec.Command("git", "-C", cat.catalogRoot, "submodule", "update", "--init", "--recursive")

	err = e.Run()
	if err != nil {
		log.Errorf("Failed to update submodules of the catalog from git repo %s, error: %v", cat.URL, err.Error())
		return err
	}
	cat.LastUpdated = time.Now().Format(time.RFC3339)
	cat.State = "active"
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
		config := make(map[string]interface{})

		//Read the config.yml file
		err = yaml.Unmarshal(yamlFile, &config)
		if err != nil {
			log.Errorf("Error unmarshalling config.yml under template: %s, error: %v", relativePath, err)
		} else {
			template.Name, _ = config["name"].(string)
			template.Category, _ = config["category"].(string)
			template.Description, _ = config["description"].(string)
			template.Version, _ = config["version"].(string)
			template.Maintainer, _ = config["maintainer"].(string)
			template.License, _ = config["license"].(string)
			template.ProjectURL, _ = config["projectURL"].(string)
			template.IsSystem, _ = config["isSystem"].(string)
			template.DefaultVersion, _ = config["version"].(string)
			template.Labels = map[string]string{}

			labels, _ := config["labels"].(map[interface{}]interface{})
			for k, v := range labels {
				template.Labels[fmt.Sprint(k)] = fmt.Sprint(v)
			}
		}
	}
}

func readRancherCompose(relativePath string, newTemplate *model.Template) error {

	composeBytes, err := readFile(relativePath, "rancher-compose.yml")
	if err != nil {
		return err
	}

	//read the questions section
	RC := make(map[string]model.RancherCompose)
	err = yaml.Unmarshal(*composeBytes, &RC)
	if err != nil {
		log.Errorf("Error unmarshalling %s under template: %s, error: %v", "rancher-compose.yml", relativePath, err)
		return err
	}
	newTemplate.Questions = RC[".catalog"].Questions
	newTemplate.Name = RC[".catalog"].Name
	newTemplate.Description = RC[".catalog"].Description
	newTemplate.Version = RC[".catalog"].Version
	newTemplate.MinimumRancherVersion = RC[".catalog"].MinimumRancherVersion
	newTemplate.Output = RC[".catalog"].Output
	newTemplate.Labels = RC[".catalog"].Labels
	binding, err := model.CreateBindings(relativePath)
	if err != nil {
		return err
	}
	newTemplate.Bindings = binding
	newTemplate.MaximumRancherVersion = RC[".catalog"].MaximumRancherVersion
	newTemplate.UpgradeFrom = RC[".catalog"].UpgradeFrom
	return nil
}

func readFile(relativePath string, fileName string) (*[]byte, error) {
	filePath := path.Join(relativePath, fileName)
	filename, err := filepath.Abs(filePath)
	if err != nil {
		log.Errorf("Error forming path to file %s, error: %v", filePath, err)
		return nil, err
	}

	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		log.Debugf("File %s does not exist", filePath)
		return nil, err
	}

	composeBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Errorf("Error reading file %s, error: %v", filePath, err)
		return nil, err
	}
	return &composeBytes, nil
}

//ExtractTemplatePrefixAndName reads the template prefix and name
func ExtractTemplatePrefixAndName(templateID string) (string, string) {
	var prefix, suffix string
	prefix = "templates"
	suffix = templateID

	if strings.Contains(templateID, "*") {
		prefix = strings.Split(templateID, "*")[0] + "-templates"
		suffix = strings.Split(templateID, "*")[1]
	}
	return prefix, suffix
}

//ReadTemplateVersion reads the template version details
func (cat *Catalog) ReadTemplateVersion(templateID string, versionID string) (*model.Template, bool) {

	prefix, templateName := ExtractTemplatePrefixAndName(templateID)
	path := cat.CatalogID + "/" + prefix + "/" + templateName + "/" + versionID
	parentPath := cat.CatalogID + "/" + templateID
	parentMetadata, ok := cat.metadata[parentPath]

	if ok {
		newTemplate := model.Template{}
		newTemplate.Path = cat.CatalogID + "/" + templateID + "/" + versionID
		newTemplate.TemplateBase = parentMetadata.TemplateBase
		newTemplate.Id = cat.CatalogID + ":" + templateID + ":" + versionID
		newTemplate.CatalogID = cat.CatalogID
		newTemplate.DefaultVersion = parentMetadata.DefaultVersion
		newTemplate.Category = parentMetadata.Category
		newTemplate.IsSystem = parentMetadata.IsSystem
		newTemplate.Files = make(map[string]string)

		foundIcon, foundReadme, err := walkVersion(CatalogRootDir+path, &newTemplate)

		if err != nil {
			log.Errorf("Error reading template at path: %s, error: %v", path, err)
			return nil, false
		}

		if !foundIcon {
			//use the parent icon
			newTemplate.IconLink = parentMetadata.IconLink
		}

		if !foundReadme {
			//use the parent readme
			newTemplate.ReadmeLink = parentMetadata.ReadmeLink
		}

		return &newTemplate, true
	}

	return nil, false

}

func walkVersion(path string, template *model.Template) (bool, bool, error) {
	dirList, err := ioutil.ReadDir(path)

	if err != nil {
		log.Errorf("Error reading template at path: %s, error: %v", path, err)
		return false, false, err
	}

	var foundIcon, foundReadme bool

	for _, subfile := range dirList {
		if strings.HasPrefix(subfile.Name(), "catalogIcon") {
			template.IconLink = template.Id + "?image"
			foundIcon = true
			PathToImage[template.Path] = subfile.Name()

		} else if strings.HasPrefix(strings.ToLower(subfile.Name()), "readme") {
			template.ReadmeLink = template.Id + "?readme"
			foundReadme = true
			PathToReadme[template.Path] = subfile.Name()

		} else {
			//read if its a file and put it in the files map
			if !subfile.IsDir() {
				bytes, err := readFile(path, subfile.Name())
				if err != nil {
					continue
				}
				key := deriveFilePath(template.Path, path) + subfile.Name()

				template.Files[key] = string(*bytes)
				if strings.HasPrefix(subfile.Name(), "rancher-compose") {
					readRancherCompose(path, template)
				}
			} else {
				//grab files under this folder
				walkVersion(path+"/"+subfile.Name(), template)
			}
		}
	}

	return foundIcon, foundReadme, nil
}

func deriveFilePath(templatePath string, path string) string {
	//template.Path = prachi/ElasticSearch/0
	//path = ./DATA/prachi/templates/ElasticSearch/0  return ""
	//path = ./DATA/prachi/templates/ElasticSearch/0/redis return redis/
	//path = ./DATA/prachi/templates/ElasticSearch/0/redis/front  return redis/front/

	lastIndexOfSlash := strings.LastIndex(templatePath, "/")
	if lastIndexOfSlash != -1 {
		version := templatePath[lastIndexOfSlash+1 : len(templatePath)]
		parts := strings.SplitAfter(path, "/"+version+"/")
		var key string
		if len(parts) > 1 {
			key = parts[1] + "/"
		}
		return key
	}

	return path + "/"
}
