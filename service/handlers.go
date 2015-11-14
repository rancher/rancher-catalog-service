package service

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/go-semver/semver"
	"github.com/gorilla/mux"
	"github.com/rancher/go-rancher/api"
	"github.com/rancher/go-rancher/client"
	"github.com/rancher/rancher-catalog-service/manager"
	"github.com/rancher/rancher-catalog-service/model"
)

const headerForwardedProto string = "X-Forwarded-Proto"

//ListCatalogs is a handler for route /catalogs and returns a collection of catalog metadata
func ListCatalogs(w http.ResponseWriter, r *http.Request) {
	catalogs := manager.ListAllCatalogs()
	resp := manager.CatalogCollection{}

	for _, value := range catalogs {
		value.CatalogLink = BuildURL(r, "catalog", value.CatalogLink)
		PopulateResource(r, "catalog", value.CatalogID, &value.Resource)
		resp.Data = append(resp.Data, value)
	}

	PopulateCollection(&resp.Collection, "catalog")
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
}

//GetCatalog is a handler for route /catalog/{catalogID} and returns the specific catalog metadata
func GetCatalog(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	catalogID := vars["catalogId"]
	catalog, ok := manager.GetCatalog(catalogID)

	if ok {
		catalog.CatalogLink = BuildURL(r, "catalog", catalog.CatalogLink)
		PopulateResource(r, "catalog", catalog.CatalogID, &catalog.Resource)
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(catalog)
	} else {
		log.Debugf("Cannot find catalog by catalogID: %s", catalogID)
		http.NotFound(w, r)
	}

}

//GetTemplatesForCatalog is a handler for listing templated under a given catalogID
func GetTemplatesForCatalog(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	catalogID := vars["catalogId"]

	if catalogID != "" {
		log.Debugf("Request to get templates for catalog %s", catalogID)
		templates := manager.ListTemplatesForCatalog(catalogID)

		rancherVersion := r.URL.Query().Get("minimumRancherVersion_lte")
		if rancherVersion != "" {
			log.Debugf("Request to get all templates under catalog %s with minimumRancherVersion <= %s", catalogID, rancherVersion)
		}

		//read the catalog
		resp := model.TemplateCollection{}
		for _, value := range templates {
			if rancherVersion != "" {
				var err error
				value.VersionLinks, err = filterByMinimumRancherVersion(rancherVersion, &value)
				if err != nil {
					//cannot apply the filter, return empty set
					break
				}
			}

			//if no versions are present then just skip the template
			if len(value.VersionLinks) == 0 {
				continue
			}

			log.Debugf("Found Template: %s", value.Name)

			value.VersionLinks = PopulateTemplateLinks(r, &value, "template")
			resp.Data = append(resp.Data, value)
		}

		api.GetApiContext(r).Write(&resp)

	}

}

//ListTemplates is a handler for route /templates and returns a collection of template metadata
func ListTemplates(w http.ResponseWriter, r *http.Request) {

	var templates []model.Template
	catalogID := r.URL.Query().Get("catalog")
	if catalogID != "" {
		log.Debugf("Request to get templates for catalog %s", catalogID)
		templates = manager.ListTemplatesForCatalog(catalogID)
	} else {
		templates = manager.ListAllTemplates()
	}

	rancherVersion := r.URL.Query().Get("minimumRancherVersion_lte")
	if rancherVersion != "" {
		log.Debugf("Request to get all templates with minimumRancherVersion <= %s", rancherVersion)
	} else {
		log.Debug("Request to list all templates in the Catalog")
	}

	//read the catalog
	resp := model.TemplateCollection{}
	for _, value := range templates {
		if rancherVersion != "" {
			var err error
			value.VersionLinks, err = filterByMinimumRancherVersion(rancherVersion, &value)
			if err != nil {
				//cannot apply the filter, return empty set
				break
			}
		}

		//if no versions are present then just skip the template
		if len(value.VersionLinks) == 0 {
			continue
		}

		log.Debugf("Found Template: %s", value.Name)

		value.VersionLinks = PopulateTemplateLinks(r, &value, "template")
		resp.Data = append(resp.Data, value)
	}

	api.GetApiContext(r).Write(&resp)
}

func filterByMinimumRancherVersion(rancherVersion string, template *model.Template) (map[string]string, error) {
	copyOfversionLinks := make(map[string]string)
	re := regexp.MustCompile(`v([a-zA-Z0-9.]+)`)
	rancherVersion = re.ReplaceAllString(rancherVersion, "$1")

	vB, err := semver.NewVersion(rancherVersion)
	if err != nil {
		log.Errorf("Error loading the passed filter minimumRancherVersion_lte with semver %s", err.Error())
		return copyOfversionLinks, err
	}

	for templateVersion, minRancherVersion := range template.TemplateVersionRancherVersion {
		if minRancherVersion != "" {
			minRancherVersion = re.ReplaceAllString(minRancherVersion, "$1")
			vA, err := semver.NewVersion(minRancherVersion)
			if err != nil {
				log.Errorf("Error loading version with semver %s", err.Error())
				continue
			}

			if minRancherVersion == rancherVersion || vA.LessThan(*vB) {
				//this template version passes the filter
				copyOfversionLinks[templateVersion] = template.VersionLinks[templateVersion]
			}
		} else {
			//no min rancher version specified, so this template works with any rancher version
			copyOfversionLinks[templateVersion] = template.VersionLinks[templateVersion]
		}
	}

	return copyOfversionLinks, nil
}

//LoadTemplateDetails returns details of the template
func LoadTemplateDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	templateIDString := vars["catalog_template_Id"]
	pathTokens := strings.Split(templateIDString, ":")

	if len(pathTokens) == 2 {
		loadTemplateMetadata(pathTokens[0], pathTokens[1], w, r)
	} else if len(pathTokens) == 3 {
		loadTemplateVersion(pathTokens[0], pathTokens[1], pathTokens[2], w, r)
	}
}

//loadTemplateMetadata returns template metadata for the provided templateId
func loadTemplateMetadata(catalogID string, templateID string, w http.ResponseWriter, r *http.Request) {
	path := catalogID + "/" + templateID
	log.Debugf("Request to load metadata for template: %s", path)
	templateMetadata, ok := manager.GetTemplateMetadata(catalogID, templateID)
	if ok {
		templateMetadata.VersionLinks = PopulateTemplateLinks(r, &templateMetadata, "template")
		api.GetApiContext(r).Write(&templateMetadata)
	} else {
		log.Debugf("Cannot find metadata for template: %s", path)
		http.NotFound(w, r)
	}
}

//loadTemplateVersion returns template version details for the provided templateId/versionId
func loadTemplateVersion(catalogID string, templateID string, versionID string, w http.ResponseWriter, r *http.Request) {
	//read the template version from disk
	path := catalogID + "/" + templateID + "/" + versionID
	log.Debugf("Request to load details for template version: %s", path)

	template, ok := manager.ReadTemplateVersion(catalogID, templateID, versionID)
	if ok {
		template.VersionLinks = PopulateTemplateLinks(r, template, "template")
		PopulateResource(r, "template", template.Id, &template.Resource)
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(template)
	} else {
		log.Debugf("Cannot find template: %s", path)
		http.NotFound(w, r)
	}
}

//LoadImage returns template image
func LoadImage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	//path := "DATA/templates/" + vars["templateId"] + "/" + vars["versionId"] + "/" + vars["imageId"]
	path := "DATA/" + vars["catalogId"] + "/templates/" + vars["templateId"] + "/" + vars["versionId"] + "/" + vars["imageId"]
	log.Debugf("Request to load Image: %s", path)
	http.ServeFile(w, r, path)
}

//LoadFile returns template image
func LoadFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	//path := "DATA/templates/" + vars["templateId"] + "/" + vars["versionId"] + "/" + vars["fileId"]
	path := "DATA/" + vars["catalogId"] + "/templates/" + vars["templateId"] + "/" + vars["versionId"] + "/" + vars["fileId"]
	log.Debugf("Request to load file: %s", path)
	http.ServeFile(w, r, path)
}

//RefreshCatalog will be doing a force catalog refresh
func RefreshCatalog(w http.ResponseWriter, r *http.Request) {
	log.Infof("Request to refresh catalog")
	manager.RefreshAllCatalogs()
}

//GetUpgradeInfo returns if any new versions are available for the given template uuid
func GetUpgradeInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	templateUUID := vars["templateUUID"]
	log.Debugf("Request to get new template versions for uuid %s", templateUUID)

	templateMetadata, ok := manager.GetNewTemplateVersions(templateUUID)
	if ok {
		log.Debugf("Template returned by uuid: %v", templateMetadata.VersionLinks)
		log.Debugf("Found Template: %s", templateMetadata.Name)
		upgradeInfo := model.UpgradeInfo{}
		upgradeInfo.CurrentVersion = templateMetadata.Version

		upgradeInfo.NewVersionLinks = make(map[string]string)
		upgradeInfo.NewVersionLinks = PopulateTemplateLinks(r, &templateMetadata, "template")
		//PopulateResource(r, "upgrade", templateMetadata.Name, &templateMetadata.Resource)
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		upgradeInfo.Type = "upgradeInfo"
		json.NewEncoder(w).Encode(upgradeInfo)
	} else {
		log.Debugf("Cannot provide upgradeInfo as, cannot find metadata for template uuid: %s", templateUUID)
		http.NotFound(w, r)
	}
}

//PopulateCollection will populate any metadata for the resource collection
func PopulateCollection(collection *client.Collection, resourceType string) {
	collection.Type = "collection"
	collection.ResourceType = resourceType
}

//PopulateTemplateLinks will populate the links needed to load a template from the service
func PopulateTemplateLinks(r *http.Request, template *model.Template, resourceType string) map[string]string {

	copyOfversionLinks := make(map[string]string)
	for key, value := range template.VersionLinks {
		copyOfversionLinks[key] = BuildURL(r, resourceType, value)
	}

	template.IconLink = BuildURL(r, "image", template.IconLink)
	if template.ReadmeLink != "" {
		template.ReadmeLink = BuildURL(r, "file", template.ReadmeLink)
	}

	return copyOfversionLinks
}

//PopulateResource will populate any metadata for the resource
func PopulateResource(r *http.Request, resourceType, resourceID string, resource *client.Resource) {
	resource.Type = resourceType

	selfLink := BuildURL(r, resourceType, resourceID)

	resource.Links = map[string]string{
		"self": selfLink,
	}
}

//BuildURL will generate the links needed for template versions/resource self links
func BuildURL(r *http.Request, resourceType, resourceID string) string {

	proto := r.Header.Get(headerForwardedProto)
	var scheme string
	if proto != "" {
		scheme = proto + "://"
	} else {
		scheme = "http://"
	}
	var host = r.Host
	var pluralName = resourceType + "s"
	var version = "v1-catalog"
	//get the url
	return scheme + host + "/" + version + "/" + pluralName + "/" + resourceID

}
