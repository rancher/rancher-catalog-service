package manager

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/rancher/go-rancher/client"
	"github.com/rancher/rancher-catalog-service/model"
)

type arrayFlags []string

//CatalogInput stores catalogs accepted from JSON file
type CatalogInput struct {
	Name    string
	RepoURL string
	Branch  string
}

func (i *arrayFlags) String() string {
	return fmt.Sprint(*i)
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var (
	refreshInterval = flag.Int64("refreshInterval", 60, "Time interval (in Seconds) to periodically pull the catalog from git repo")
	logFile         = flag.String("logFile", "", "Log file")
	debug           = flag.Bool("debug", false, "Debug")
	validate        = flag.Bool("validate", false, "Validate catalog yaml and exit")
	configFile      = flag.String("configFile", "", "Config file")

	// Port is the listen port of the HTTP server
	Port              = flag.Int("port", 8088, "HTTP listen port")
	refreshReqChannel = make(chan int, 1)
	//CatalogsCollection is the map storing template catalogs
	CatalogsCollection map[string]*Catalog
	//CatalogReadyChannel signals if the catalog is cloned and loaded in memmory
	CatalogReadyChannel = make(chan int, 1)

	//PathToImage holds the mapping between a template path in the repo to its image name
	PathToImage map[string]string
	//PathToReadme holds the mapping between a template path in the repo to its readme file name
	PathToReadme map[string]string

	//ValidationMode is used to determine if code is just checking Yaml syntax
	ValidationMode bool

	catalogURL     arrayFlags
	commandLineURL arrayFlags

	catalogURLBranch string

	//URLBranchMap aps repo url to branch
	URLBranchMap map[string]string

	reloadChan = make(chan chan error)
)

//CatalogRootDir is the root folder under which all catalogs are cloned
const CatalogRootDir string = "./DATA/"

//WatchSignals handles SIGHUP
func WatchSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	go func() {
		for _ = range c {
			log.Info("Received HUP signal")
			SetEnv()
			go Init()
		}
	}()
}

//GetCommandLine parses the command line args
func GetCommandLine() {
	flag.Var(&catalogURL, "catalogUrl", "git repo url in the form repo_id=repo_url. Specify the flag multiple times for multiple repos")

	flag.Parse()
	commandLineURL = catalogURL
	SetEnv()
}

//SetEnv parses the command line args and sets the necessary variables
func SetEnv() {

	catalogURL = catalogURL[:0]
	catalogURL = commandLineURL

	var catalogObject []CatalogInput
	var catalogObjectJSON []CatalogInput
	URLBranchMap = make(map[string]string)

	//NEW CODE
	// If catalog provided through command line
	if len(catalogURL) > 0 {
		for i := 0; i < len(catalogURL); i++ {
			obj := CatalogInput{}
			obj.RepoURL = catalogURL[i]
			obj.Branch = "master"
			catalogObject = append(catalogObject, obj)
			URLBranchMap[obj.RepoURL] = obj.Branch
		}
	}

	if *configFile != "" {
		libraryContent, err := ioutil.ReadFile(*configFile)
		if err != nil {
			log.Debugf("JSON file does not exist, continuing for command line URLs")
		} else {
			err = json.Unmarshal(libraryContent, &catalogObjectJSON)
			if err != nil {
				log.Errorf("JSON data format invalid, error : %v\n", err)
			}

			for _, value := range catalogObjectJSON {
				if (CatalogInput{} != value) {
					catalogObject = append(catalogObject, value)
					value.RepoURL = value.Name + "=" + value.RepoURL
					catalogURL = append(catalogURL, value.RepoURL)
					URLBranchMap[value.RepoURL] = value.Branch
				}
			}
		}
	}

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	if *validate {
		ValidationMode = true
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
	if catalogURL != nil {
		CatalogsCollection = make(map[string]*Catalog)
		PathToImage = make(map[string]string)
		PathToReadme = make(map[string]string)

		defaultFound := false

		for _, value := range catalogURL {
			catalogURLBranch = URLBranchMap[value]

			value = strings.TrimSpace(value)
			if value != "" {
				urls := strings.Split(value, ",")
				for _, singleURL := range urls {
					tokens := strings.Split(singleURL, "=")
					if len(tokens) == 1 {
						//add a default catalogName
						if defaultFound {
							log.Fatalf("Please specify a catalog name for %s", tokens[0])
						}
						defaultFound = true
						tokens = append(tokens, tokens[0])
						tokens[0] = "library"
					}
					newCatalog := Catalog{}
					newCatalog.CatalogID = tokens[0]
					url := tokens[1]
					index := strings.Index(tokens[1], "://")
					if index != -1 {
						//lowercase the scheme
						url = strings.ToLower(tokens[1][:index]) + tokens[1][index:]
					}
					if catalogURLBranch != "" {
						newCatalog.URLBranch = catalogURLBranch
					}
					newCatalog.URL = url
					refChan := make(chan int, 1)
					newCatalog.refreshReqChannel = &refChan
					newCatalog.catalogRoot = CatalogRootDir + tokens[0]
					CatalogsCollection[tokens[0]] = &newCatalog
					log.Infof("Using catalog %s=%s", tokens[0], url)
				}
				// Init()
			}
		}
	} else {
		err := "Halting Catalog service, Catalog git repo url not provided"
		log.Fatal(err)
		_ = fmt.Errorf(err)
	}
}

//Init clones or pulls the catalog, starts background refresh thread
func Init() {
	for _, catalog := range CatalogsCollection {
		catalog.readCatalog()
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
	for catalogID, cat := range CatalogsCollection {
		catalog := Catalog{
			Resource: client.Resource{
				Id:   catalogID,
				Type: "catalog",
			},
		}
		catalog.CatalogID = catalogID
		catalog.CatalogLink = catalogID + "/templates"
		catalog.State = cat.State
		catalog.URL = cat.URL
		catalog.URLBranch = cat.URLBranch
		catalog.Message = cat.Message
		catalog.LastUpdated = cat.LastUpdated
		catalogCollection = append(catalogCollection, catalog)
	}
	return catalogCollection
}

//GetCatalog gets the metadata of the specified catalog
func GetCatalog(catalogID string) (Catalog, bool) {
	cat, ok := CatalogsCollection[catalogID]
	catalog := Catalog{
		Resource: client.Resource{
			Type: "catalog",
		},
	}
	if ok {
		catalog.Id = cat.CatalogID
		catalog.CatalogID = cat.CatalogID
		catalog.State = cat.State
		catalog.URL = cat.URL
		catalog.URLBranch = cat.URLBranch
		catalog.Message = cat.Message
		catalog.LastUpdated = cat.LastUpdated
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
	cat, ok := CatalogsCollection[catalogID]
	if !ok {
		return model.Template{}, ok
	}
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
func GetNewTemplateVersions(path string) (model.Template, bool) {
	templateMetadata := model.Template{}

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
		log.Debugf("Error %v reading Current Version from path: %s", err, path)
	} else {
		templateMetadata, ok := cat.metadata[catalogID+"/"+parentPath]
		if ok {
			log.Debugf("Template found by path: %s", path)
			copyOfversionLinks := make(map[string]string)
			for key, value := range templateMetadata.VersionLinks {
				if value != path {
					otherVersionTokens := strings.Split(value, ":")
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

	log.Debugf("Template metadata not found by path: %s", path)
	return templateMetadata, false
}
