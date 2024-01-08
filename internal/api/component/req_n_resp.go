package component

// ComponentInstance indicates the component be added into proposal
// It is generated from the Component object, and includes the data filled in proposal
type ComponentInstance struct {
	ID            uint   `json:"id"`
	ComponentId   uint   `json:"component_id"`
	ComponentName string `json:"name"`
	Schema        string `json:"schema"`
	Data          string `json:"data"`

	CreateTs int64 `json:"create_ts"`
}
