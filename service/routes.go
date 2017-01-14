package service

import (
	"net/http"
	"strconv"

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
	httpWrapper.Router.ServeHTTP(w, r)
}

//ReturnHTTPError handles sending out CatalogError response
func ReturnHTTPError(w http.ResponseWriter, r *http.Request, httpStatus int, errorMessage string) {
	w.WriteHeader(httpStatus)

	err := model.CatalogError{
		Resource: client.Resource{
			Type: "error",
		},
		Status:  strconv.Itoa(httpStatus),
		Message: errorMessage,
	}

	api.CreateApiContext(w, r, schemas)
	api.GetApiContext(r).Write(&err)

}

//Route defines the properties of a go mux http route
type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

var schemas *client.Schemas

//Routes array of Route defined
type Routes []Route

//NewRouter creates and configures a mux router
func NewRouter() *mux.Router {
	schemas = &client.Schemas{}

	// ApiVersion
	apiVersion := schemas.AddType("apiVersion", client.Resource{})
	apiVersion.CollectionMethods = []string{}

	// Schema
	schemas.AddType("schema", client.Schema{})

	// Question
	question := schemas.AddType("question", model.Question{})
	question.CollectionMethods = []string{}

	schemas.AddType("output", model.Output{})

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
	f := templateVersion.ResourceFields["questions"]
	f.Type = "array[question]"
	templateVersion.ResourceFields["questions"] = f

	// Catalog
	catalog := schemas.AddType("catalog", manager.Catalog{})
	delete(catalog.ResourceFields, "catalogLink")

	// Error
	err := schemas.AddType("error", model.CatalogError{})
	err.CollectionMethods = []string{}

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
	router.GetRoute("RefreshCatalogTemplates").Queries("action", "refresh")

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
	Route{
		"RefreshCatalogTemplates",
		"POST",
		"/v1-catalog/catalogs/{catalogId}/templates",
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
