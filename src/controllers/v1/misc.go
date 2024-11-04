package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cloud01-wu/cgsl/httpx/model"
)

func responseError(w http.ResponseWriter, resultObject *model.Response, statusCode int, err error) error {
	var oErr = err

	resultObject.Errors = append(resultObject.Errors, model.Error{
		Status: statusCode,
		Detail: err.Error(),
	})

	result, err := json.Marshal(resultObject)
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintln(w, string(result))
	}

	return oErr
}

func GetStringFromQuery(query url.Values, key string, fallback string) string {
	value := query.Get(key)
	if value == "" {
		return fallback
	}
	return value
}

func GetIntFromQuery(query url.Values, key string, fallback int) int {
	s := query.Get(key)
	if value, err := strconv.Atoi(s); err == nil {
		return value
	}
	return fallback
}

func GetBoolFromQuery(query url.Values, key string, fallback bool) bool {
	s := query.Get(key)
	if value, err := strconv.ParseBool(s); err == nil {
		return value
	}
	return fallback
}

func GetStringArrayFromQuery(query url.Values, key string) []string {
	values := query[key]
	return values
}

type QueryFilter struct {
	Column string
	Value  string
}

func GetFiltersFromQuery(query url.Values, key string) []QueryFilter {
	values := query[key]
	filters := []QueryFilter{}

	for _, value := range values {
		tokens := strings.Split(value, ":")
		if len(tokens) != 2 {
			continue
		}

		filters = append(filters, QueryFilter{
			Column: tokens[0],
			Value:  tokens[1],
		})
	}

	return filters
}
