package nostr

type WhiteList struct {
	IP        string `json:"ip"`
	PubKey    string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

type BlackList struct {
	IP        string `json:"ip"`
	PubKey    string `json:"pub_key"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

type ListRepo interface {
	StoreWhiteList(wl *WhiteList) error
	GetWhiteList(pbk string) (*WhiteList, error)
	GetWhiteLists(pbks []string) ([]*WhiteList, error)
	GetWLIPS() ([]string, error)
	StoreBlackList(bl *BlackList) error
	GetBlackList(ip string) (*BlackList, error)
	GetBlackLists(ips []string) ([]*BlackList, error)
	GetBLIPS() ([]string, error)
}
