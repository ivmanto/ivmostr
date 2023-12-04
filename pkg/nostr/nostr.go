/*
TODO: Add suitable license
*/
package nostr

type RelayInfo struct {
	Name           string                   `json:"name"`
	Description    string                   `json:"description"`
	Pubkey         string                   `json:"pubkey"`
	Contact        string                   `json:"contact"`
	SupportedNIPS  []int                    `json:"supported_nips"`
	Software       string                   `json:"software"`
	Version        string                   `json:"version"`
	Limitations    *Limitations             `json:"limitations"`
	Retentions     []map[string]interface{} `json:"retention"`
	RelayCountries []string                 `json:"relay_countries"`
}

type Limitations struct {
	MaxMessageLength    int   `json:"max_message_length"`
	MaxSubscriptions    int   `json:"max_subscriptions"`
	MaxFilters          int   `json:"max_filters"`
	MaxLimit            int   `json:"max_limit"`
	MaxSubIdLength      int   `json:"max_subid_length"`
	MaxEventTags        int   `json:"max_event_tags"`
	MaxContentLength    int   `json:"max_content_length"`
	MinPowDifficulty    int   `json:"min_pow_difficulty"`
	AuthRequired        bool  `json:"auth_required"`
	PaymentRequired     bool  `json:"payment_required"`
	RestrictedWrites    bool  `json:"restricted_writes"`
	CreatedAtLowerLimit int64 `json:"created_at_lower_limit"`
	CreatedAtUpperLimit int   `json:"created_at_upper_limit"`
}
