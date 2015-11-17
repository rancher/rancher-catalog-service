package service

import (
	"net/http"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/go-semver/semver"
	"github.com/gorilla/mux"
	"github.com/rancher/go-rancher/api"
	"github.com/rancher/rancher-catalog-service/manager"
	"github.com/rancher/rancher-catalog-service/model"
)

const headerForwardedProto string = "X-Forwarded-Proto"

//ListCatalogs is a handler for route /catalogs and returns a collection of catalog metadata
func ListCatalogs(w http.ResponseWriter, r *http.Request) {
	apiContext := api.GetApiContext(r)

	catalogs := manager.ListAllCatalogs()
	resp := manager.CatalogCollection{}

	for _, value := range catalogs {
		if value.Links == nil {
			value.Links = map[string]string{}
		}

		value.Links["templates"] = apiContext.UrlBuilder.ReferenceByIdLink("catalog", value.CatalogLink)
		resp.Data = append(resp.Data, value)
	}

	apiContext.Write(&resp)
}

//GetCatalog is a handler for route /catalog/{catalogID} and returns the specific catalog metadata
func GetCatalog(w http.ResponseWriter, r *http.Request) {
	apiContext := api.GetApiContext(r)

	vars := mux.Vars(r)
	catalogID := vars["catalogId"]
	catalog, ok := manager.GetCatalog(catalogID)

	if ok {
		catalog.CatalogLink = apiContext.UrlBuilder.ReferenceByIdLink("catalog", catalog.CatalogLink)
		apiContext.Write(&catalog)
	} else {
		log.Debugf("Cannot find catalog by catalogID: %s", catalogID)
		http.NotFound(w, r)
	}

}

//GetTemplatesForCatalog is a handler for listing templated under a given catalogID
func GetTemplatesForCatalog(w http.ResponseWriter, r *http.Request) {
	apiContext := api.GetApiContext(r)

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

			value.VersionLinks = PopulateTemplateLinks(r, &value)
			resp.Data = append(resp.Data, value)
		}

		apiContext.Write(&resp)
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

		log.Debugf("Found Template: %s", value.Id)
		value.VersionLinks = PopulateTemplateLinks(r, &value)
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
	templateIDString := vars["catalog_template_version_Id"]
	pathTokens := strings.Split(templateIDString, ":")

	var catalogID, templateID, versionID string

	if len(pathTokens) == 2 {
		catalogID = pathTokens[0]
		templateID = pathTokens[1]
	} else if len(pathTokens) == 3 {
		catalogID = pathTokens[0]
		templateID = pathTokens[1]
		versionID = pathTokens[2]
	} else {
		log.Debugf("Cannot find metadata for template Id: %s", templateIDString)
		http.NotFound(w, r)
	}

	//check if request is to get image or readme file
	imageID := r.URL.Query().Get("image")
	if imageID != "" {
		loadFile(catalogID, templateID, versionID, imageID, w, r)
		return
	}
	fileID := r.URL.Query().Get("readme")
	if fileID != "" {
		loadFile(catalogID, templateID, versionID, fileID, w, r)
		return
	}

	if versionID != "" {
		loadTemplateVersion(catalogID, templateID, versionID, w, r)
	} else {
		loadTemplateMetadata(catalogID, templateID, w, r)
	}
}

//loadTemplateMetadata returns template metadata for the provided templateId
func loadTemplateMetadata(catalogID string, templateID string, w http.ResponseWriter, r *http.Request) {
	path := catalogID + "/" + templateID
	log.Debugf("Request to load metadata for template: %s", path)
	templateMetadata, ok := manager.GetTemplateMetadata(catalogID, templateID)
	if ok {
		PopulateTemplateLinks(r, &templateMetadata)
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
		template.Type = "templateVersion"
		template.VersionLinks = PopulateTemplateLinks(r, template)
		template.UUID = template.Id
		upgradeInfo := GetUpgradeInfo(r, template.Path)
		template.UpgradeVersionLinks = upgradeInfo.NewVersionLinks
		api.GetApiContext(r).Write(&template)
	} else {
		log.Debugf("Cannot find template: %s", path)
		http.NotFound(w, r)
	}
}

//LoadFile returns template image
func loadFile(catalogID string, templateID string, versionID string, fileID string, w http.ResponseWriter, r *http.Request) {
	path := "DATA/" + catalogID + "/templates/" + templateID + "/" + versionID + "/" + fileID
	log.Debugf("Request to load file: %s", path)
	http.ServeFile(w, r, path)
}

//RefreshCatalog will be doing a force catalog refresh
func RefreshCatalog(w http.ResponseWriter, r *http.Request) {
	log.Infof("Request to refresh catalog")
	manager.RefreshAllCatalogs()
}

//GetUpgradeInfo returns if any new versions are available for the given template uuid
func GetUpgradeInfo(r *http.Request, path string) model.UpgradeInfo {
	var upgradeInfo model.UpgradeInfo
	log.Debugf("Request to get new template versions for path %s", path)

	templateMetadata, ok := manager.GetNewTemplateVersions(path)
	if ok {
		log.Debugf("Template returned by path: %v", templateMetadata.VersionLinks)
		log.Debugf("Found Template: %s", templateMetadata.Name)
		upgradeInfo.CurrentVersion = templateMetadata.Version

		upgradeInfo.NewVersionLinks = make(map[string]string)
		upgradeInfo.NewVersionLinks = PopulateTemplateLinks(r, &templateMetadata)
	} else {
		log.Debugf("Cannot provide upgradeInfo as, cannot find metadata for template path: %s", path)
	}

	return upgradeInfo
}

//PopulateTemplateLinks will populate the links needed to load a template from the service
func PopulateTemplateLinks(r *http.Request, template *model.Template) map[string]string {
	if template.Links == nil {
		template.Links = map[string]string{}
	}

	apiContext := api.GetApiContext(r)

	copyOfversionLinks := make(map[string]string)
	for key, value := range template.VersionLinks {
		copyOfversionLinks[key] = apiContext.UrlBuilder.ReferenceByIdLink("template", value)
	}

	template.Links["icon"] = apiContext.UrlBuilder.ReferenceByIdLink("images", template.IconLink)
	if template.ReadmeLink != "" {
		template.Links["readme"] = apiContext.UrlBuilder.ReferenceByIdLink("files", template.ReadmeLink)
	}
	if template.ProjectURL != "" {
		template.Links["project"] = template.ProjectURL
	}

	template.VersionLinks = copyOfversionLinks
	template.DefaultVersion = template.Version

	return copyOfversionLinks
}
