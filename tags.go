package xcore

// TagContent contains the data for an m.tag message type
// https://matrix.org/docs/spec/client_server/r0.4.0.html#m-tag
type TagContent struct {
	Tags map[string]TagProperties `json:"tags"`
}

// TagProperties contains the properties of a Tag
type TagProperties struct {
	Order float32 `json:"order,omitempty"` // Empty values must be neglected
}
