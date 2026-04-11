package httpserver

type graphPayload struct {
	Nodes []nodePayload `json:"nodes"`
	Edges []edgePayload `json:"edges"`
}

type nodePayload struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type edgePayload struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Positive bool   `json:"positive"`
}

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Messages []string `json:"messages,omitempty"`
	Error    string   `json:"error,omitempty"`
}
