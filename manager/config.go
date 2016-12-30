package manager

import (
	"encoding/json"
	"io/ioutil"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/satori/go.uuid"
)

//Config stores catalog service configuration
type Config struct {
	Catalogs    map[string]*CatalogInput `json:"catalogs"`
	AccessToken uuid.UUID                `json:"accessToken"`
}

//CatalogInput references a git repository
type CatalogInput struct {
	URL    string `json:"url"`
	Branch string `json:"branch"`
}

func newConfig(catalogs []string, filename string) *Config {
	c := &Config{
		Catalogs: make(map[string]*CatalogInput),
	}

	c.loadCommandline(catalogs)

	if filename != "" {
		c.loadFile(filename)
		if c.AccessToken == uuid.Nil {
			c.AccessToken = uuid.NewV4()
		}
		for _, catalog := range c.Catalogs {
			if catalog.Branch == "" {
				catalog.Branch = "master"
			}
		}
		c.storeFile(*configFile)
	}

	return c
}

func (c *Config) loadCommandline(catalogs []string) {
	// If catalog provided through command line
	defaultFound := false
	for _, urls := range catalogURL {
		for _, url := range strings.Split(strings.TrimSpace(urls), ",") {
			tokens := strings.Split(url, "=")
			if len(tokens) == 1 {
				if defaultFound {
					log.Fatalf("Please specify a repo_id for %s", tokens[0])
				}
				defaultFound = true
				tokens = append(tokens, tokens[0])
				tokens[0] = "library"
			}
			if len(tokens) != 2 {
				log.Warnf("Ignoring malformed catalog url: %s", url)
			} else {
				c.Catalogs[tokens[0]] = &CatalogInput{
					URL:    tokens[1],
					Branch: "master",
				}
			}
		}
	}
}

func (c *Config) loadFile(filename string) {
	if file, err := ioutil.ReadFile(filename); err != nil {
		log.Warnf("JSON read file error: %v", err)
	} else {
		// unmarshalling is best-effort, log a warning and continue
		if err = json.Unmarshal(file, c); err != nil {
			log.Warnf("JSON unmarshal error: %v", err)
		}
	}
}

func (c *Config) storeFile(filename string) {
	if d, err := json.MarshalIndent(c, "", "  "); err != nil {
		log.Warnf("Couldn't marshal configuration JSON")
	} else {
		ioutil.WriteFile(filename, d, 0666)
	}
}

func (i *CatalogInput) toCatalog(id string, uuid string) *Catalog {
	c := &Catalog{}
	c.CatalogID = id
	index := strings.Index(i.URL, "://")
	if index != -1 {
		//lowercase the scheme
		i.URL = strings.ToLower(i.URL[:index]) + i.URL[index:]
	}
	if strings.Contains(i.URL, "git.rancher.io") {
		i.URL = strings.Join([]string{i.URL, uuid}, "/")
	}
	if i.Branch != "" {
		c.URLBranch = i.Branch
	}
	c.URL = i.URL
	refChan := make(chan int, 1)
	c.refreshReqChannel = &refChan
	c.catalogRoot = CatalogRootDir + id
	return c
}
