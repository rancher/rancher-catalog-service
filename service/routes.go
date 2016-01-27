package service

import (
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/rancher/go-rancher/api"
	"github.com/rancher/go-rancher/client"
	"github.com/rancher/rancher-catalog-service/manager"
	"github.com/rancher/rancher-catalog-service/model"
)

//MuxWrapper is a wrapper over the mux router that returns 503 until catalog is ready
type MuxWrapper struct {
	IsReady bool
	Router  *mux.Router
}

func (httpWrapper *MuxWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case <-manager.CatalogReadyChannel:
		httpWrapper.IsReady = true
	default:
	}

	if httpWrapper.IsReady {
		//delegate to the mux router
		httpWrapper.Router.ServeHTTP(w, r)
	} else {
		log.Debugf("Service Unavailable")
		httpWrapper.returnCode503(w, r)
	}
}

func (httpWrapper *MuxWrapper) returnCode503(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("Catalog Service is not yet available, please try again later"))
}

//Route defines the properties of a go mux http route
type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

//Routes array of Route defined
type Routes []Route

//NewRouter creates and configures a mux router
func NewRouter() *mux.Router {
	schemas := &client.Schemas{}

	// ApiVersion
	apiVersion := schemas.AddType("apiVersion", client.Resource{})
	apiVersion.CollectionMethods = []string{}

	// Schema
	schemas.AddType("schema", client.Schema{})

	// Question
	question := schemas.AddType("question", model.Question{})
	question.CollectionMethods = []string{}

	// Template
	template := schemas.AddType("template", model.Template{})
	refreshAction := client.Action{}
	tempActions := make(map[string]client.Action)
	tempActions["refresh"] = refreshAction
	template.CollectionActions = tempActions

	delete(template.ResourceFields, "rancherCompose")
	delete(template.ResourceFields, "dockerCompose")
	delete(template.ResourceFields, "uuid")
	delete(template.ResourceFields, "questions")
	delete(template.ResourceFields, "TemplateVersionRancherVersion")
	delete(template.ResourceFields, "iconLink")
	delete(template.ResourceFields, "readmeLink")
	delete(template.ResourceFields, "projectURL")
	delete(template.ResourceFields, "version")

	// Template Version
	templateVersion := schemas.AddType("templateVersion", model.Template{})
	templateVersion.CollectionMethods = []string{}

	// Catalog
	catalog := schemas.AddType("catalog", manager.Catalog{})
	delete(catalog.ResourceFields, "catalogLink")

	// API framework routes
	router := mux.NewRouter().StrictSlash(true)

	router.Methods("GET").Path("/").Handler(api.VersionsHandler(schemas, "v1-catalog"))
	router.Methods("GET").Path("/v1-catalog/schemas").Handler(api.SchemasHandler(schemas))
	router.Methods("GET").Path("/v1-catalog/schemas/{id}").Handler(api.SchemaHandler(schemas))
	router.Methods("GET").Path("/v1-catalog").Handler(api.VersionHandler(schemas, "v1-catalog"))

	// Application routes

	for _, route := range routes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(api.ApiHandler(schemas, route.HandlerFunc))
	}

	router.GetRoute("RefreshCatalog").Queries("action", "refresh")

	return router
}

var routes = Routes{
	Route{
		"ListTemplates",
		"GET",
		"/v1-catalog/templates",
		ListTemplates,
	},
	Route{
		"LoadTemplateDetails",
		"GET",
		"/v1-catalog/templates/{catalog_template_version_Id}",
		LoadTemplateDetails,
	},
	Route{
		"",
		"GET",
		"/v1-catalog/templateversions/{catalog_template_version_Id}",
		LoadTemplateDetails,
	},
	Route{
		"RefreshCatalog",
		"POST",
		"/v1-catalog/templates",
		RefreshCatalog,
	},
	//http://<server_ip>:8088/v1/upgrades/<template_uuid>
	Route{
		"ListCatalogs",
		"GET",
		"/v1-catalog/catalogs",
		ListCatalogs,
	},
	Route{
		"GetCatalog",
		"GET",
		"/v1-catalog/catalogs/{catalogId}",
		GetCatalog,
	},
	Route{
		"GetTemplatesForCatalog",
		"GET",
		"/v1-catalog/catalogs/{catalogId}/templates",
		GetTemplatesForCatalog,
	},
}
