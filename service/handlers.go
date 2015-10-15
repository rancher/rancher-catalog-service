package service

import (
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/rancher/go-rancher/client"
	"github.com/rancher/rancher-catalog-service/manager"
	"github.com/rancher/rancher-catalog-service/model"
	"net/http"
)

func ListTemplates(w http.ResponseWriter, r *http.Request) {
	//read the catalog

	log.Debug("Request to list Templates Catalog")
	resp := model.TemplateCollection{}
	for _, value := range manager.Catalog {
		log.Debugf("Found Template: %s", value.Name)
		value.VersionLinks = PopulateTemplateLinks(r, &value, "template")
		PopulateResource(r, "template", value.Name, &value.Resource)
		resp.Data = append(resp.Data, value)
	}

	PopulateCollection(&resp.Collection)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)

}

func LoadTemplateMetadata(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := vars["templateId"]
	log.Debugf("Request to load metadata for template: %s", path)
	templateMetadata, ok := manager.Catalog[path]
	if ok {
		templateMetadata.VersionLinks = PopulateTemplateLinks(r, &templateMetadata, "template")
		PopulateResource(r, "template", templateMetadata.Name, &templateMetadata.Resource)
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(templateMetadata)
	} else {
		log.Debugf("Cannot find metadata for template: %s", path)
		http.NotFound(w, r)
	}
}

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

func LoadImage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := "DATA/templates/" + vars["templateId"] + "/" + vars["versionId"] + "/" + vars["imageId"]
	log.Debugf("Request to load Image: %s", path)
	http.ServeFile(w, r, path)
}

func RefreshCatalog(w http.ResponseWriter, r *http.Request) {
	log.Infof("Request to refresh catalog")
	manager.RefreshCatalog()
}

func GetUpgradeInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	templateUUID := vars["templateUUID"]
	log.Infof("Request to get new template versions for uuid %s", templateUUID)
	
	templateMetadata := manager.GetNewTemplateVersions(templateUUID)
	if templateMetadata.Name != "" {
		log.Debugf("Template returned by uuid: %v", templateMetadata.VersionLinks)
		log.Debugf("Found Template: %s", templateMetadata.Name)
		upgradeInfo := model.UpgradeInfo{}
		upgradeInfo.CurrentVersion = templateMetadata.Version
		
		upgradeInfo.NewVersionLinks =  make(map[string]string) 
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

func PopulateCollection(collection *client.Collection) {
	collection.Type = "collection"
	collection.ResourceType = "template"
}

func PopulateTemplateLinks(r *http.Request, template *model.Template, resourceType string) map[string]string {

	copyOfversionLinks := make(map[string]string)
	for key, value := range template.VersionLinks {
		copyOfversionLinks[key] = BuildURL(r, resourceType, value)
	}

	template.IconLink = BuildURL(r, "image", template.IconLink)

	return copyOfversionLinks
}

func PopulateResource(r *http.Request, resourceType, resourceId string, resource *client.Resource) {
	resource.Type = resourceType

	selfLink := BuildURL(r, resourceType, resourceId)

	resource.Links = map[string]string{
		"self": selfLink,
	}
}

func BuildURL(r *http.Request, resourceType, resourceId string) string {

	var scheme string = "http://"
	var host string = r.Host
	var pluralName string = resourceType + "s"
	var version string = "v1"
	//get the url
	return scheme + host + "/" + version + "/" + pluralName + "/" + resourceId

}
