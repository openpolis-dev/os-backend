package component

// ComponentInstance indicates the component be added into proposal
// It is generated from the Component object, and includes the data filled in proposal
type ComponentInstance struct {
	ID          uint   `json:"id"`
	ComponentId string `json:"component_id"`
	Schema      string `json:"schema"`
	Data        string `json:"data"`

	CreateTs int64 `json:"create_ts"`
	UpdateTs int64 `json:"update_ts"`
}
