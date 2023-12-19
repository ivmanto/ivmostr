package arweave

// TODO: rework all below to match the requrements from Arweave

import (
	"github.com/everFinance/goar"
)

type Arweave struct {
	db *goar.Client
}

func NewArweave() *Arweave {
	return &Arweave{}
}
