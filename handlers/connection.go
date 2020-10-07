package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/ksensehq/eventnative/adapters"
	"github.com/ksensehq/eventnative/logging"
	"github.com/ksensehq/eventnative/middleware"
	"net/http"
)

type ConnectionConfig struct {
	DestinationType  string      `json:"type"`
	ConnectionConfig interface{} `json:"config"`
}

type ConnectionTestHandler struct {
}

type RedshiftConfig struct {
	DbConfig *adapters.DataSourceConfig `json:"database"`
	S3Config *adapters.S3Config         `json:"s3"`
}

type SnowflakeExternalConfig struct {
	SnowflakeConfig adapters.SnowflakeConfig `json:"snowflake"`
	S3Config        adapters.S3Config        `json:"s3"`
}

func testConnection(config ConnectionConfig) error {
	switch config.DestinationType {
	case "postgres":
		var postgresConfig adapters.DataSourceConfig
		body, err := json.Marshal(config.ConnectionConfig)
		if err != nil {
			return err
		}
		err = json.Unmarshal(body, &postgresConfig)
		if err != nil {
			return err
		}
		if err := postgresConfig.Validate(); err != nil {
			return err
		}
		postgres, err := adapters.NewPostgres(context.Background(), &postgresConfig)
		if err != nil {
			return err
		}
		defer postgres.Close()
		return postgres.Test()
	case "clickhouse":
		var chConfig adapters.ClickHouseConfig
		body, err := json.Marshal(config.ConnectionConfig)
		if err != nil {
			return err
		}
		err = json.Unmarshal(body, &chConfig)
		if err != nil {
			return err
		}
		if err = chConfig.Validate(); err != nil {
			return err
		}
		tableStatementFactory, err := adapters.NewTableStatementFactory(&chConfig)
		if err != nil {
			return err
		}
		nonNullFields := map[string]bool{"eventn_ctx_event_id": true, "_timestamp": true}
		for i := range chConfig.Dsns {
			var resultError error
			resultError = nil
			ch, err := adapters.NewClickHouse(context.Background(), chConfig.Dsns[i], chConfig.Database, chConfig.Cluster, chConfig.Tls, tableStatementFactory, nonNullFields)
			if err != nil {
				resultError = err
				continue
			}
			resultError = ch.Test()
			if err = ch.Close(); err != nil {
				logging.Warn("Failed to close clickhouse datasource %s", err)
			}
			if resultError == nil {
				return nil
			}
		}
		return nil
	case "redshift":
		var rsConfig RedshiftConfig
		body, err := json.Marshal(config.ConnectionConfig)
		if err != nil {
			return err
		}
		err = json.Unmarshal(body, &rsConfig)
		if err != nil {
			return err
		}
		if err = rsConfig.DbConfig.Validate(); err != nil {
			return err
		}
		if rsConfig.S3Config != nil {
			if err = rsConfig.S3Config.Validate(); err != nil {
				return err
			}
		}
		redshift, err := adapters.NewAwsRedshift(context.Background(), rsConfig.DbConfig, rsConfig.S3Config)
		if err != nil {
			return err
		}
		defer redshift.Close()
		return redshift.Test()
	default:
		return errors.New("unsupported destination type " + config.DestinationType)
	}
}

func NewConnectionTestHandler() *ConnectionTestHandler {
	return &ConnectionTestHandler{}
}

func (h *ConnectionTestHandler) Handler(c *gin.Context) {
	connectionConfig := ConnectionConfig{}
	if err := c.BindJSON(&connectionConfig); err != nil {
		c.JSON(http.StatusBadRequest, middleware.ErrorResponse{Message: "Failed to parse body", Error: err})
		return
	}
	err := testConnection(connectionConfig)
	if err != nil {
		c.JSON(http.StatusBadRequest, middleware.ErrorResponse{Message: "Failed to test connection", Error: err})
		return
	}
	c.JSON(http.StatusOK, middleware.OkResponse{Status: "Connection established"})
}
