package config

var (
	User                  string
	Group                 string
	ApiVersion            string
	Port                  int
	Bind                  string
	Dev                   bool
	Debug                 bool
	DropPrivileges        bool
	UI                    bool
	ModulesDir            string
	ModulesScanSec        int
	Backend               string
	CORSOrigins           string
	FallbackProxyUrl      string
	NoCache               bool
	CachePrefixes         string
	ProxyPrefixes         string
	CacheByFullRequestURI bool
	CacheMaxAge           int64
	ImportProxiedReleases bool
	JwtSecret             string
	TlsCertPath           string
	TlsKeyPath            string
	JwtTokenPath          string
)
