package mysql

import "time"

type SecretReference struct {
	File string `yaml:"file" json:"file"`
	Env  string `yaml:"env" json:"env"`
}

type Config struct {
	Engine                 string          `yaml:"engine" json:"engine"`
	Host                   string          `yaml:"host" json:"host"`
	Port                   int             `yaml:"port" json:"port"`
	Database               string          `yaml:"database" json:"database"`
	Username               string          `yaml:"username" json:"username"`
	Password               SecretReference `yaml:"password" json:"password"`
	Socket                 string          `yaml:"socket" json:"socket"`
	TLSMode                string          `yaml:"tls_mode" json:"tls_mode"`
	ConnectTimeout         time.Duration   `yaml:"connect_timeout" json:"connect_timeout"`
	SingleTransaction      bool            `yaml:"single_transaction" json:"single_transaction"`
	Quick                  bool            `yaml:"quick" json:"quick"`
	Routines               bool            `yaml:"routines" json:"routines"`
	Triggers               bool            `yaml:"triggers" json:"triggers"`
	Events                 bool            `yaml:"events" json:"events"`
	IncludeDatabases       []string        `yaml:"include_databases" json:"include_databases"`
	ExcludeTables          []string        `yaml:"exclude_tables" json:"exclude_tables"`
	IncludeUsers           bool            `yaml:"include_users" json:"include_users"`
	NonTransactionalPolicy string          `yaml:"non_transactional_policy" json:"non_transactional_policy"`
	ExtraOptions           []string        `yaml:"extra_options" json:"extra_options"`
}

func DefaultConfig(engine string) Config {
	if engine == "" {
		engine = "mysql"
	}
	return Config{Engine: engine, Port: 3306, TLSMode: "preferred", ConnectTimeout: 10 * time.Second, SingleTransaction: true, Quick: true, Routines: true, Triggers: true, Events: true, NonTransactionalPolicy: "warn"}
}
