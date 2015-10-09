package model

//import "github.com/rancher/rancher-compose/rancher"

type Question struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Type        string   `json:"type" yaml:"type"`
	Options     []string `json:"options" yaml:"options"`
}

type RancherCompose struct {
	//rancher.RancherConfig	`yaml:",inline"`
	Questions []Question `json:"questions" yaml:"questions,omitempty"`
}
