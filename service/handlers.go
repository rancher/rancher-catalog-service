package service

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/blang/semver"
	"github.com/gorilla/mux"
	"github.com/rancher/go-rancher/api"
	"github.com/rancher/rancher-catalog-service/manager"
	"github.com/rancher/rancher-catalog-service/model"
)

const headerForwardedProto string = "X-Forwarded-Proto"

var (
	re = regexp.MustCompile(`v([a-zA-Z0-9.]+)`)
)

//ListCatalogs is a handler for route /catalogs and returns a collection of catalog metadata
func ListCatalogs(w http.ResponseWriter, r *http.Request) {
	apiContext := api.GetApiContext(r)

	catalogs := manager.ListAllCatalogs()
	resp := manager.CatalogCollection{}

	for _, value := range catalogs {
		if value.Links == nil {
			value.Links = map[string]string{}
		}

		value.Links["templates"] = URLEncoded(apiContext.UrlBuilder.ReferenceByIdLink("catalog", value.CatalogLink))
		value.Links["self"] = URLEncoded(apiContext.UrlBuilder.ReferenceByIdLink("catalog", value.Id))
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
		catalog.CatalogLink = URLEncoded(apiContext.UrlBuilder.ReferenceByIdLink("catalog", catalog.CatalogLink))
		if catalog.Links == nil {
			catalog.Links = map[string]string{}
		}
		catalog.Links["self"] = URLEncoded(apiContext.UrlBuilder.ReferenceByIdLink("catalog", catalog.Id))
		apiContext.Write(&catalog)
	} else {
		log.Debugf("Cannot find catalog by catalogID: %s", catalogID)
		ReturnHTTPError(w, r, http.StatusNotFound, fmt.Sprintf("Cannot find catalog by catalogID: %s", catalogID))
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

		rancherVersionGte := r.URL.Query().Get("maximumRancherVersion_gte")
		if rancherVersionGte != "" {
			log.Debugf("Request to get all templates under catalog %s with maximumRancherVersion >= %s", catalogID, rancherVersionGte)
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

			if rancherVersionGte != "" {
				var err error
				value.VersionLinks, err = filterByMaximumRancherVersion(rancherVersionGte, &value)
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

		resp.Actions = make(map[string]string)
		resp.Actions["refresh"] = api.GetApiContext(r).UrlBuilder.ReferenceByIdLink("template", "") + "?action=refresh"
		apiContext.Write(&resp)
	}

}

//ListTemplates is a handler for route /templates and returns a collection of template metadata
func ListTemplates(w http.ResponseWriter, r *http.Request) {

	var templates []model.Template
	catalogID := r.URL.Query().Get("catalog")

	if catalogID == "" {
		catalogID = r.URL.Query().Get("catalogId")
	}

	if catalogID != "" {
		log.Debugf("Request to get templates for catalog %s", catalogID)
		templates = manager.ListTemplatesForCatalog(catalogID)
	} else {
		log.Debugf("Request to get templates from all catalogs ")
		templates = manager.ListAllTemplates()
	}

	//List the filters

	templateBaseEq := r.URL.Query().Get("templateBase_eq")
	if templateBaseEq != "" {
		log.Debugf("And only get %s templates ", templateBaseEq)
	}
	templateBaseNe := r.URL.Query().Get("templateBase_ne")
	if templateBaseNe != "" {
		log.Debugf("And not the %s templates ", templateBaseNe)
	}

	rancherVersion := r.URL.Query().Get("minimumRancherVersion_lte")
	if rancherVersion != "" {
		log.Debugf("And templates with minimumRancherVersion <= %s", rancherVersion)
	}

	rancherVersionGte := r.URL.Query().Get("maximumRancherVersion_gte")
	if rancherVersionGte != "" {
		log.Debugf("And templates with maximumRancherVersion >= %s", rancherVersionGte)
	}

	category := r.URL.Query().Get("category_ne")
	if category != "" {
		log.Debugf("And templates with category not = %s", category)
	}

	//read the catalog
	resp := model.TemplateCollection{}
	for _, value := range templates {
		if templateBaseEq != "" && !strings.Contains(value.Id, templateBaseEq+"*") {
			continue
		}

		if templateBaseNe != "" && strings.Contains(value.Id, templateBaseNe+"*") {
			continue
		}

		if rancherVersion != "" {
			var err error
			value.VersionLinks, err = filterByMinimumRancherVersion(rancherVersion, &value)
			if err != nil {
				//cannot apply the filter, return empty set
				break
			}
		}

		if rancherVersionGte != "" {
			var err error
			value.VersionLinks, err = filterByMaximumRancherVersion(rancherVersionGte, &value)
			if err != nil {
				//cannot apply the filter, return empty set
				break
			}
		}

		//if no versions are present then just skip the template
		if len(value.VersionLinks) == 0 {
			continue
		}

		if category != "" && value.Category != "" {
			if strings.EqualFold(category, value.Category) {
				//skip the templates matching the category_ne filter
				continue
			}
		}

		log.Debugf("Found Template: %s", value.Id)
		value.VersionLinks = PopulateTemplateLinks(r, &value)
		resp.Data = append(resp.Data, value)
	}

	resp.Actions = make(map[string]string)
	resp.Actions["refresh"] = api.GetApiContext(r).UrlBuilder.ReferenceByIdLink("template", "") + "?action=refresh"
	api.GetApiContext(r).Write(&resp)
}

func filterByMinimumRancherVersion(rancherVersion string, template *model.Template) (map[string]string, error) {
	copyOfversionLinks := make(map[string]string)

	vB, err := getSemVersion(rancherVersion)
	if err != nil {
		log.Errorf("Error loading the passed filter minimumRancherVersion_lte with semver %s", err.Error())
		return copyOfversionLinks, err
	}

	for templateVersion, minRancherVersion := range template.TemplateVersionRancherVersion {
		if minRancherVersion != "" {
			vA, err := getSemVersion(minRancherVersion)
			if err != nil {
				log.Errorf("Error loading version with semver %s", err.Error())
				continue
			}

			if minRancherVersion == rancherVersion || vA.LT(*vB) {
				//this template version passes the filter
				if template.VersionLinks[templateVersion] != "" {
					copyOfversionLinks[templateVersion] = template.VersionLinks[templateVersion]
				}
			}
		} else {
			//no min rancher version specified, so this template works with any rancher version
			if template.VersionLinks[templateVersion] != "" {
				copyOfversionLinks[templateVersion] = template.VersionLinks[templateVersion]
			}
		}
	}

	return copyOfversionLinks, nil
}

func filterByMaximumRancherVersion(rancherVersion string, template *model.Template) (map[string]string, error) {
	copyOfversionLinks := make(map[string]string)

	vB, err := getSemVersion(rancherVersion)
	if err != nil {
		log.Errorf("Error loading the passed filter maximumRancherVersion_gte with semver %s", err.Error())
		return copyOfversionLinks, err
	}

	for templateVersion, maxRancherVersion := range template.TemplateVersionRancherVersionGte {
		if maxRancherVersion != "" {
			vA, err := getSemVersion(maxRancherVersion)
			if err != nil {
				log.Errorf("Error loading version with semver %s", err.Error())
				continue
			}

			if maxRancherVersion == rancherVersion || vA.GT(*vB) {
				//this template version passes the filter
				if template.VersionLinks[templateVersion] != "" {
					copyOfversionLinks[templateVersion] = template.VersionLinks[templateVersion]
				}
			}
		} else {
			//no max rancher version specified, so this template works with any rancher version
			if template.VersionLinks[templateVersion] != "" {
				copyOfversionLinks[templateVersion] = template.VersionLinks[templateVersion]
			}
		}
	}

	return copyOfversionLinks, nil
}

func getSemVersion(versionStr string) (*semver.Version, error) {
	versionStr = re.ReplaceAllString(versionStr, "$1")

	semVersion, err := semver.Make(versionStr)
	if err != nil {
		log.Errorf("Error %v loading semver for version string %s", err.Error(), versionStr)
		return nil, err
	}
	return &semVersion, nil
}

func isMinRancherVersionLTE(templateMinRancherVersion string, rancherVersion string) (bool, error) {
	vA, err := getSemVersion(templateMinRancherVersion)
	if err != nil {
		log.Errorf("Error loading template minRancherVersion %s with semver %s", templateMinRancherVersion, err.Error())
		return false, err
	}

	vB, err := getSemVersion(rancherVersion)
	if err != nil {
		log.Errorf("Error loading the passed filter minimumRancherVersion_lte %s with semver %s", rancherVersion, err.Error())
		return false, err
	}

	if templateMinRancherVersion == rancherVersion || vA.LT(*vB) {
		return true, nil
	}
	return false, nil
}

func isMaxRancherVersionGTE(templateMaxRancherVersion string, rancherVersion string) (bool, error) {
	vA, err := getSemVersion(templateMaxRancherVersion)
	if err != nil {
		log.Errorf("Error loading template maxRancherVersion %s with semver %s", templateMaxRancherVersion, err.Error())
		return false, err
	}

	vB, err := getSemVersion(rancherVersion)
	if err != nil {
		log.Errorf("Error loading the passed filter maximumRancherVersion_gte %s with semver %s", rancherVersion, err.Error())
		return false, err
	}

	if templateMaxRancherVersion == rancherVersion || vA.GT(*vB) {
		return true, nil
	}
	return false, nil
}

//LoadTemplateDetails returns details of the template
func LoadTemplateDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	templateIDString := vars["catalog_template_version_Id"]
	log.Debugf("LoadTemplateDetails for template Id: %s", templateIDString)
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
		ReturnHTTPError(w, r, http.StatusNotFound, fmt.Sprintf("Cannot find metadata for template Id: %s", templateIDString))
	}

	if r.URL.RawQuery != "" && strings.EqualFold("image", r.URL.RawQuery) {
		callToRead(catalogID, templateID, versionID, w, r)
		loadFile(catalogID, templateID, versionID, manager.PathToImage, w, r)
		return
	}

	if r.URL.RawQuery != "" && strings.EqualFold("readme", r.URL.RawQuery) {
		callToRead(catalogID, templateID, versionID, w, r)
		loadFile(catalogID, templateID, versionID, manager.PathToReadme, w, r)
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
	tempID := catalogID + ":" + templateID
	log.Debugf("Request to load metadata for template: %s", path)
	rancherVersion := r.URL.Query().Get("minimumRancherVersion_lte")
	if rancherVersion != "" {
		log.Debugf("only versions with minimumRancherVersion <= %s", rancherVersion)
	}
	rancherVersionGte := r.URL.Query().Get("maximumRancherVersion_gte")
	if rancherVersionGte != "" {
		log.Debugf("only versions with maximumRancherVersion >= %s", rancherVersionGte)
	}
	templateMetadata, ok := manager.GetTemplateMetadata(catalogID, templateID)
	if ok {
		if rancherVersion != "" {
			var err error
			templateMetadata.VersionLinks, err = filterByMinimumRancherVersion(rancherVersion, &templateMetadata)
			if err != nil {
				log.Debugf("Cannot apply the minimumRancherVersion_lte filter for template: %s", path)
				ReturnHTTPError(w, r, http.StatusNotFound, fmt.Sprintf("Cannot apply the minimumRancherVersion_lte filter for template: %s", tempID))
			}
		}

		if rancherVersionGte != "" {
			var err error
			templateMetadata.VersionLinks, err = filterByMaximumRancherVersion(rancherVersionGte, &templateMetadata)
			if err != nil {
				log.Debugf("Cannot apply the maximumRancherVersion_gte filter for template: %s", path)
				ReturnHTTPError(w, r, http.StatusNotFound, fmt.Sprintf("Cannot apply the maximumRancherVersion_gte filter for template: %s", tempID))
			}
		}
		PopulateTemplateLinks(r, &templateMetadata)
		api.GetApiContext(r).Write(&templateMetadata)
	} else {
		log.Debugf("Cannot find metadata for template: %s", path)
		ReturnHTTPError(w, r, http.StatusNotFound, fmt.Sprintf("Cannot find metadata for template: %s", tempID))
	}
}

func callToRead(catalogID string, templateID string, versionID string, w http.ResponseWriter, r *http.Request) {
	path := catalogID + "/" + templateID + "/" + versionID
	log.Debugf("Request to load  template version: %s", path)
	rancherVersion := r.URL.Query().Get("minimumRancherVersion_lte")
	if rancherVersion != "" {
		log.Debugf("and if minimumRancherVersion <= %s", rancherVersion)
	}
	rancherVersionGte := r.URL.Query().Get("maximumRancherVersion_gte")
	if rancherVersionGte != "" {
		log.Debugf("and if maximumRancherVersion >= %s", rancherVersionGte)
	}

	manager.ReadTemplateVersion(catalogID, templateID, versionID)
}

//loadTemplateVersion returns template version details for the provided templateId/versionId
func loadTemplateVersion(catalogID string, templateID string, versionID string, w http.ResponseWriter, r *http.Request) {
	//read the template version from disk
	tempVersionID := catalogID + ":" + templateID + ":" + versionID
	path := catalogID + "/" + templateID + "/" + versionID
	log.Debugf("Request to load  template version: %s", path)
	rancherVersion := r.URL.Query().Get("minimumRancherVersion_lte")
	if rancherVersion != "" {
		log.Debugf("and if minimumRancherVersion <= %s", rancherVersion)
	}
	rancherVersionGte := r.URL.Query().Get("maximumRancherVersion_gte")
	if rancherVersionGte != "" {
		log.Debugf("and if maximumRancherVersion >= %s", rancherVersionGte)
	}

	template, ok := manager.ReadTemplateVersion(catalogID, templateID, versionID)
	if ok {
		template.Type = "templateVersion"
		template.VersionLinks = PopulateTemplateLinks(r, template)
		upgradeInfo := GetUpgradeInfo(r, template.Path)
		template.UpgradeVersionLinks = upgradeInfo.NewVersionLinks
		api.GetApiContext(r).Write(&template)
	} else {
		log.Debugf("Cannot find template: %s", path)
		ReturnHTTPError(w, r, http.StatusNotFound, fmt.Sprintf("Cannot find template: %s", tempVersionID))
	}
}

//loadFile loads the file under the catalog
func loadFile(catalogID string, templateID string, versionID string, fileNameMap map[string]string, w http.ResponseWriter, r *http.Request) {
	var fileID, path string

	prefix, templateName := manager.ExtractTemplatePrefixAndName(templateID)

	if versionID != "" {
		var ok bool
		fileID, ok = fileNameMap[catalogID+"/"+templateID+"/"+versionID]
		if !ok {
			fileID = fileNameMap[catalogID+"/"+templateID]
			path = "DATA/" + catalogID + "/" + prefix + "/" + templateName + "/" + fileID
		} else {
			path = "DATA/" + catalogID + "/" + prefix + "/" + templateName + "/" + versionID + "/" + fileID
		}
	} else {
		fileID = fileNameMap[catalogID+"/"+templateID]
		path = "DATA/" + catalogID + "/" + prefix + "/" + templateName + "/" + fileID
	}
	log.Debugf("Request to load file: %s", path)
	http.ServeFile(w, r, path)
}

//RefreshCatalog will be doing a force catalog refresh
func RefreshCatalog(w http.ResponseWriter, r *http.Request) {
	log.Infof("Request to refresh catalog")

	//Reload catalog
	manager.SetEnv()
	manager.Init()

	manager.RefreshAllCatalogs()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

//GetUpgradeInfo returns if any new versions are available for the given template uuid
func GetUpgradeInfo(r *http.Request, path string) model.UpgradeInfo {
	var upgradeInfo model.UpgradeInfo
	log.Debugf("Request to get new template versions available for upgrade, for path %s", path)
	rancherVersion := r.URL.Query().Get("minimumRancherVersion_lte")
	if rancherVersion != "" {
		log.Debugf("and with minimumRancherVersion <= %s", rancherVersion)
	}
	rancherVersionGte := r.URL.Query().Get("maximumRancherVersion_gte")
	if rancherVersionGte != "" {
		log.Debugf("and if maximumRancherVersion >= %s", rancherVersionGte)
	}

	templateMetadata, ok := manager.GetNewTemplateVersions(path)
	if ok {
		if rancherVersion != "" {
			var err error
			templateMetadata.VersionLinks, err = filterByMinimumRancherVersion(rancherVersion, &templateMetadata)
			if err != nil {
				log.Debugf("Cannot provide upgradeInfo as cannot apply the minimumRancherVersion_lte filter for template: %s", path)
				return upgradeInfo
			}
		}
		if rancherVersionGte != "" {
			var err error
			templateMetadata.VersionLinks, err = filterByMaximumRancherVersion(rancherVersionGte, &templateMetadata)
			if err != nil {
				log.Debugf("Cannot provide upgradeInfo as cannot apply the maximumRancherVersion_gte filter for template: %s", path)
				return upgradeInfo
			}
		}
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
		copyOfversionLinks[key] = URLEncoded(apiContext.UrlBuilder.ReferenceByIdLink("template", value))
	}

	template.Links["icon"] = URLEncoded(apiContext.UrlBuilder.ReferenceByIdLink("template", template.IconLink))
	if template.ReadmeLink != "" {
		template.Links["readme"] = URLEncoded(apiContext.UrlBuilder.ReferenceByIdLink("template", template.ReadmeLink))
	}
	if template.ProjectURL != "" {
		template.Links["project"] = template.ProjectURL
	}
	template.Links["self"] = URLEncoded(apiContext.UrlBuilder.ReferenceByIdLink("template", template.Id))

	template.VersionLinks = copyOfversionLinks

	return copyOfversionLinks
}

//URLEncoded encodes the urls so that spaces are allowed in resource names
func URLEncoded(str string) string {
	u, err := url.Parse(str)
	if err != nil {
		log.Errorf("Error encoding the url: %s , error: %v", str, err)
		return str
	}
	return u.String()
}
