package main

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

func NewDB(config Config) (*gorm.DB, error) {
	dialectors := map[string]gorm.Dialector{
		"mysql":     mysql.Open(config.DbDsn),
		"sqlite":    sqlite.Open(config.DbDsn),
		"postgres":  postgres.Open(config.DbDsn),
		"sqlserver": sqlserver.Open(config.DbDsn),
	}

	dialector, ok := dialectors[config.DbType]
	if !ok {
		return nil, fmt.Errorf("unsupported database type: %s", config.DbType)
	}

	return gorm.Open(dialector, &gorm.Config{})
}
