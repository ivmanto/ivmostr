package config

type ServiceConfig struct {
	Name                string   `yaml:"name"`
	ProjectID           string   `yaml:"project_id"`
	Port                string   `yaml:"port"`
	Relay_Host          string   `yaml:"relay_host"`
	Relay_access        string   `yaml:"relay_access"`
	WsioopTimeOut       int      `yaml:"ws_io_operation_timeout"`
	PoolMaxWorkers      int      `yaml:"pool_max_workers"`
	PoolQueue           int      `yaml:"pool_queue"`
	MaxConnsPerIP       int      `yaml:"max_conns_per_ip"`
	MaxErrsPerConn      int      `yaml:"max_errs_per_conn"`
	CloudLoggingEnabled bool     `yaml:"cloud_logging_enabled"`
	TrustedOrigins      []string `yaml:"trusted_origins"`
	Firestore           *firestore
}

type firestore struct {
	ProjectID               string `yaml:"project_id"`
	DefaultLimit            int    `yaml:"default_limit"`
	EventsCollectioNname    string `yaml:"events_collection_name"`
	WhiteListCollectionName string `yaml:"whitelist_collection_name"`
	BlackListCollectionName string `yaml:"blacklist_collection_name"`
}

func (s *ServiceConfig) GetProjectID() string {
	f := s.Firestore
	return f.ProjectID
}

// GetDLV - gets the default limit value set in the firstore service configuration
func (s *ServiceConfig) GetDLV() int {
	f := s.Firestore
	if f.DefaultLimit < 1 {
		return 20
	}
	return f.DefaultLimit
}

func (s *ServiceConfig) GetEventsCollectionName() string {
	f := s.Firestore
	return f.EventsCollectioNname
}

func (s *ServiceConfig) GetWhiteListCollectionName() string {
	f := s.Firestore
	return f.WhiteListCollectionName
}

func (s *ServiceConfig) GetBlackListCollectionName() string {
	f := s.Firestore
	return f.BlackListCollectionName
}

func (s *ServiceConfig) GetTrustedOrigins() []string {
	return s.TrustedOrigins
}

// [ ] TODO: (on demand) implement more methods to get the configuration attributes
