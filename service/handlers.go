package service

import (
	"encoding/json"
	"net/http"
	"regexp"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/go-semver/semver"
	"github.com/gorilla/mux"
	"github.com/rancher/go-rancher/api"
	"github.com/rancher/go-rancher/client"
	"github.com/rancher/rancher-catalog-service/manager"
	"github.com/rancher/rancher-catalog-service/model"
)

const headerForwardedProto string = "X-Forwarded-Proto"

//ListTemplates is a handler for route /templates and returns a collection of template metadata
func ListTemplates(w http.ResponseWriter, r *http.Request) {

	rancherVersion := r.URL.Query().Get("minimumRancherVersion_lte")
	if rancherVersion != "" {
		log.Debugf("Request to get all templates with minimumRancherVersion <= %s", rancherVersion)
	} else {
		log.Debug("Request to list All Templates in the Catalog")
	}

	//read the catalog
	resp := model.TemplateCollection{}
	for _, value := range manager.Catalog {

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

//LoadTemplateMetadata returns template metadata for the provided templateId
func LoadTemplateMetadata(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := vars["templateId"]
	log.Debugf("Request to load metadata for template: %s", path)
	templateMetadata, ok := manager.Catalog[path]
	if ok {
		templateMetadata.VersionLinks = PopulateTemplateLinks(r, &templateMetadata, "template")
		api.GetApiContext(r).Write(&templateMetadata)
	} else {
		log.Debugf("Cannot find metadata for template: %s", path)
		http.NotFound(w, r)
	}
}

//LoadTemplateVersion returns template version details for the provided templateId/versionId
func LoadTemplateVersion(w http.ResponseWriter, r *http.Request) {
	//read the template version from disk
	vars := mux.Vars(r)
	path := vars["templateId"] + "/" + vars["versionId"]
	log.Debugf("Request to load details for template version: %s", path)

	template := manager.ReadTemplateVersion(path)
	template.VersionLinks = PopulateTemplateLinks(r, &template, "template")
	PopulateResource(r, "template", template.Path, &template.Resource)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(template)
}

//LoadImage returns template image
func LoadImage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := "DATA/templates/" + vars["templateId"] + "/" + vars["versionId"] + "/" + vars["imageId"]
	log.Debugf("Request to load Image: %s", path)
	http.ServeFile(w, r, path)
}

//LoadFile returns template image
func LoadFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := "DATA/templates/" + vars["templateId"] + "/" + vars["versionId"] + "/" + vars["fileId"]
	log.Debugf("Request to load file: %s", path)
	http.ServeFile(w, r, path)
}

//RefreshCatalog will be doing a force catalog refresh
func RefreshCatalog(w http.ResponseWriter, r *http.Request) {
	log.Infof("Request to refresh catalog")
	manager.RefreshCatalog()
}

//GetUpgradeInfo returns if any new versions are available for the given template uuid
func GetUpgradeInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	templateUUID := vars["templateUUID"]
	log.Infof("Request to get new template versions for uuid %s", templateUUID)

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
func PopulateCollection(collection *client.Collection) {
	collection.Type = "collection"
	collection.ResourceType = "template"
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
