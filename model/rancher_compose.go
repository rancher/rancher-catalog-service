package model

//import "github.com/rancher/rancher-compose/rancher"

//Question holds the properties of a question present in rancher-compose.yml file
type Question struct {
	Variable     string   `json:"variable" yaml:"variable"`
	Label        string   `json:"label" yaml:"label"`
	Description  string   `json:"description" yaml:"description"`
	Type         string   `json:"type" yaml:"type"`
	Required     bool     `json:"required" yaml:"required"`
	Default      string   `json:"default" yaml:"default"`
	Group        string   `json:"group" yaml:"group"`
	MinLength    int      `json:"minLength" yaml:"minLength"`
	MaxLength    int      `json:"maxLength" yaml:"maxLength"`
	Min          int      `json:"min" yaml:"min"`
	Max          int      `json:"max" yaml:"max"`
	Options      []string `json:"options" yaml:"options"`
	ValidChars   string   `json:"validChars" yaml:"validChars"`
	InvalidChars string   `json:"invalidChars" yaml:"invalidChars"`
}

//RancherCompose holds the questions array
type RancherCompose struct {
	//rancher.RancherConfig	`yaml:",inline"`
	Questions []Question `json:"questions" yaml:"questions,omitempty"`
}
