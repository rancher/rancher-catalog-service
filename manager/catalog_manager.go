package manager

import (
	"flag"
	"fmt"
	//"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-catalog-service/model"
)

var (
	catalogURLList  = flag.String("catalogUrlList", "", "Comma separated list providing a git repo id and git repo urls (such as a public GitHub repo url) in the form of id1:url1,id2:url2... ")
	refreshInterval = flag.Int64("refreshInterval", 60, "Time interval (in Seconds) to periodically pull the catalog from git repo")
	logFile         = flag.String("logFile", "", "Log file")
	debug           = flag.Bool("debug", false, "Debug")
	// Port is the listen port of the HTTP server
	Port              = flag.Int("port", 8088, "HTTP listen port")
	refreshReqChannel = make(chan int, 1)
	//CatalogsCollection is the map storing template catalogs
	CatalogsCollection map[string]*Catalog
	//CatalogReadyChannel signals if the catalog is cloned and loaded in memmory
	CatalogReadyChannel = make(chan int, 1)
	//UUIDToPath holds the mapping between a template UUID to the path in the repo
	UUIDToPath map[string]string
)

//CatalogRootDir is the root folder under which all catalogs are cloned
const CatalogRootDir string = "./DATA/"

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

	if *catalogURLList == "" {
		err := "Halting Catalog service, Catalog git repo url not provided"
		log.Fatal(err)
		_ = fmt.Errorf(err)
	} else {
		urlList := strings.TrimSpace(*catalogURLList)
		//parse the comma separated list into catalog structs
		urls := strings.Split(urlList, ",")
		CatalogsCollection = make(map[string]*Catalog)
		UUIDToPath = make(map[string]string)

		for _, value := range urls {
			value = strings.TrimSpace(value)
			if value != "" {
				catalogProps := strings.SplitN(value, ":", 2)
				newCatalog := Catalog{}
				newCatalog.CatalogID = catalogProps[0]
				newCatalog.url = catalogProps[1]
				refChan := make(chan int, 1)
				newCatalog.refreshReqChannel = &refChan
				newCatalog.catalogRoot = CatalogRootDir + catalogProps[0]
				CatalogsCollection[catalogProps[0]] = &newCatalog
			}
		}
	}
}

//Init clones or pulls the catalog, starts background refresh thread
func Init() {
	_, err := os.Stat(CatalogRootDir)
	if !os.IsNotExist(err) || err == nil {
		//remove the existing repo
		err := os.RemoveAll(CatalogRootDir)
		if err != nil {
			log.Fatal("Cannot remove the existing catalog data folder ./DATA/", err)
			_ = fmt.Errorf("Cannot remove the existing catalog data folder ./DATA/, error: " + err.Error())
		}
	} else {
		log.Info("./DATA/ folder does not exist, proceeding to clone the repo : ", err)
	}

	for _, catalog := range CatalogsCollection {
		catalog.cloneCatalog()
	}

	//start a background timer to pull from the Catalog periodically
	startCatalogBackgroundPoll()
	CatalogReadyChannel <- 1
}

func startCatalogBackgroundPoll() {
	ticker := time.NewTicker(time.Duration(*refreshInterval) * time.Second)
	go func() {
		for t := range ticker.C {
			log.Debugf("Running background Catalog Refresh Thread at time %s", t)
			RefreshAllCatalogs()
		}
	}()
}

//RefreshAllCatalogs refreshes the catalogs by syncing changes from github
func RefreshAllCatalogs() {
	for _, catalog := range CatalogsCollection {
		log.Debugf("Refreshing catalog %s", catalog.getID())
		catalog.refreshCatalog()
	}
}

//ListAllCatalogs lists the catalog id's and links
func ListAllCatalogs() []Catalog {
	var catalogCollection []Catalog
	for catalogID := range CatalogsCollection {
		catalog := Catalog{}
		catalog.CatalogID = catalogID
		catalog.CatalogLink = catalogID + "/templates"
		catalogCollection = append(catalogCollection, catalog)
	}
	return catalogCollection
}

//GetCatalog gets the metadata of the specified catalog
func GetCatalog(catalogID string) (Catalog, bool) {
	cat, ok := CatalogsCollection[catalogID]
	catalog := Catalog{}
	if ok {
		catalog.CatalogID = cat.CatalogID
		catalog.CatalogLink = cat.CatalogID + "/templates"
	}
	return catalog, ok
}

//ListAllTemplates lists the templates from all catalogs
func ListAllTemplates() []model.Template {
	var metadataCollection []model.Template
	for _, catalog := range CatalogsCollection {
		for _, template := range catalog.metadata {
			metadataCollection = append(metadataCollection, template)
		}
	}
	return metadataCollection
}

//ListTemplatesForCatalog lists the templates from the given catalog
func ListTemplatesForCatalog(catalogID string) []model.Template {
	var metadataCollection []model.Template
	cat, ok := CatalogsCollection[catalogID]
	if ok {
		for _, template := range cat.metadata {
			metadataCollection = append(metadataCollection, template)
		}
	}
	return metadataCollection
}

//GetTemplateMetadata gets the metadata of the specified template from the given catalog
func GetTemplateMetadata(catalogID string, templateID string) (model.Template, bool) {
	cat := CatalogsCollection[catalogID]
	template, ok := cat.metadata[catalogID+"/"+templateID]
	return template, ok
}

//ReadTemplateVersion reads the details of a template version
func ReadTemplateVersion(catalogID string, templateID string, versionID string) (*model.Template, bool) {
	cat, ok := CatalogsCollection[catalogID]
	if ok {
		return cat.ReadTemplateVersion(templateID, versionID)
	}
	return nil, ok
}

//GetNewTemplateVersions gets new versions of a template if available
func GetNewTemplateVersions(templateUUID string) (model.Template, bool) {
	templateMetadata := model.Template{}
	path := UUIDToPath[templateUUID]
	if path != "" {
		//find the base template metadata name
		tokens := strings.Split(path, "/")
		catalogID := tokens[0]
		parentPath := tokens[1]
		cVersion := tokens[2]

		//refresh the catalog and sync any new changes
		cat := CatalogsCollection[catalogID]
		//cat.refreshCatalog()

		currentVersion, err := strconv.Atoi(cVersion)

		if err != nil {
			log.Debugf("Error %v reading Current Version from path: %s for uuid: %s", err, path, templateUUID)
		} else {
			templateMetadata, ok := cat.metadata[catalogID+"/"+parentPath]
			if ok {
				log.Debugf("Template found by uuid: %s", templateUUID)
				copyOfversionLinks := make(map[string]string)
				for key, value := range templateMetadata.VersionLinks {
					if value != path {
						otherVersionTokens := strings.Split(value, "/")
						oVersion := otherVersionTokens[2]
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
