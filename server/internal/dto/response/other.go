package response

type GaodeIPResponse struct {
	Status   string `json:"status"`
	Info     string `json:"info"`
	Province string `json:"province"`
	City     string `json:"city"`
}