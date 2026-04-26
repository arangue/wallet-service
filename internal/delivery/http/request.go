package http

const (
	maxOwnerIDLen   = 255
	maxReferenceLen = 255
	maxAmountLen    = 50
)

type createWalletRequest struct {
	OwnerID string `json:"owner_id"`
}

type depositRequest struct {
	Amount    string `json:"amount"`
	Reference string `json:"reference"`
}

type withdrawRequest struct {
	Amount    string `json:"amount"`
	Reference string `json:"reference"`
}
