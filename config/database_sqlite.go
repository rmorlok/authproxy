package config

type DatabaseSqlite struct {
	Provider DatabaseProvider `json:"provider" yaml:"provider"`
	Path     string           `json:"path" yaml:"path"`
}

func (d *DatabaseSqlite) GetProvider() DatabaseProvider {
	return DatabaseProviderSqlite
}
