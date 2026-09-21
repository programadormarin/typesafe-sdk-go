package typesafe

// ModelMetadata describes a single available model returned by the Models endpoint.
type ModelMetadata struct {
	// Name is the model identifier accepted in a request's model field.
	Name string `json:"name"`

	// Description is a human-readable summary of the model and its capabilities.
	Description string `json:"description"`

	// ReleaseDate is the model release date formatted as YYYY-MM-DD.
	ReleaseDate string `json:"release_date"`
}

// ListModelsResponse is returned by [Client.Models].
type ListModelsResponse struct {
	// Models is the list of models available to the account.
	Models []ModelMetadata `json:"models"`
}
