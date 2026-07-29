package domain

type Descriptor struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	Status        string   `json:"status"`
	Capabilities  []string `json:"capabilities"`
	Documentation *string  `json:"documentationUrl"`
}
