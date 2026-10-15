package api

import (
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/experiment"
	"github.com/gin-gonic/gin"
)

const maxRequestBytes = 1 << 20

type sourceRequest struct {
	Type       string `json:"type"`
	DatasetID  string `json:"dataset_id"`
	LocationID string `json:"location_id"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	Timezone   string `json:"timezone,omitempty"`
}
type outputOptions struct {
	LimitIntervals int  `json:"limit_intervals,omitempty"`
	IncludeLedger  bool `json:"include_ledger,omitempty"`
}
type singleRequest struct {
	DataSource *sourceRequest `json:"data_source"`
	Config     *config.Input  `json:"config"`
	Options    outputOptions  `json:"options,omitempty"`
}
type variationRequest struct {
	Name   string        `json:"name"`
	Config *config.Input `json:"config"`
}
type comparisonRequest struct {
	DataSource *sourceRequest     `json:"data_source"`
	BaseConfig *config.Input      `json:"base_config"`
	Variations []variationRequest `json:"variations"`
}

func decodeBody(c *gin.Context, target any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
	raw, err := io.ReadAll(c.Request.Body)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return failure{status: 413, code: "BODY_TOO_LARGE", message: "Request body exceeds 1 MiB."}
	}
	if err != nil {
		return invalid("", "Could not read the request body.")
	}
	return decodeRequest(raw, reflect.ValueOf(target).Elem())
}

func sourceQuery(source *sourceRequest) (data.Query, error) {
	if source == nil {
		return data.Query{}, invalid("data_source", "Data source is required.")
	}
	if source.Type != "gridstatus" {
		return data.Query{}, invalid("data_source.type", "Choose gridstatus.")
	}
	query := data.Query{DatasetID: source.DatasetID, LocationID: source.LocationID, StartDate: source.StartDate, EndDate: source.EndDate, Clock: source.Timezone}
	if err := validateQuery(query); err != nil {
		return data.Query{}, err
	}
	if query.Clock == "" {
		query.Clock = "market"
	}
	return query, nil
}

func validateQuery(query data.Query) error {
	for _, field := range []struct{ name, value string }{{"dataset_id", query.DatasetID}, {"location_id", query.LocationID}} {
		if strings.TrimSpace(field.value) == "" {
			return invalid("data_source."+field.name, "A nonempty value is required.")
		}
	}
	if query.Clock != "" && query.Clock != "market" && query.Clock != "UTC" {
		return invalid("data_source.timezone", "Choose market or UTC.")
	}
	start, err := time.Parse(time.DateOnly, query.StartDate)
	if err != nil || start.Year() < 1 || start.Format(time.DateOnly) != query.StartDate {
		return invalid("data_source.start_date", "Enter a valid YYYY-MM-DD date.")
	}
	end, err := time.Parse(time.DateOnly, query.EndDate)
	if err != nil || end.Year() < 1 || end.Format(time.DateOnly) != query.EndDate {
		return invalid("data_source.end_date", "Enter a valid YYYY-MM-DD date.")
	}
	if !start.Before(end) || end.After(start.AddDate(0, 0, 60)) || query.EndDate > time.Now().UTC().Format(time.DateOnly) {
		return invalid("data_source.end_date", "End date must follow start, be within 60 days, and not exceed today in UTC.")
	}
	return nil
}

func runOptions(query data.Query, options outputOptions) (experiment.Options, error) {
	result := experiment.Options{LimitIntervals: options.LimitIntervals, Clock: query.Clock}
	return result, experiment.ValidateOptions(result)
}
