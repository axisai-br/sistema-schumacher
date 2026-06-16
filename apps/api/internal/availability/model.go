package availability

import "time"

type SearchFilter struct {
	Origin            string
	Destination       string
	OriginStopID      string `json:"origin_stop_id,omitempty"`
	DestinationStopID string `json:"destination_stop_id,omitempty"`
	RouteID           string `json:"route_id,omitempty"`
	TripDate          *time.Time
	DateFrom          *time.Time
	DateTo            *time.Time
	PackageName       string
	Qty               int
	Limit             int
	OnlyActive        bool
	IncludePast       bool
}

type SearchResult struct {
	SegmentID              string  `json:"segment_id"`
	TripID                 string  `json:"trip_id"`
	RouteID                string  `json:"route_id"`
	BoardStopID            string  `json:"board_stop_id"`
	AlightStopID           string  `json:"alight_stop_id"`
	OriginStopID           string  `json:"origin_stop_id"`
	DestinationStopID      string  `json:"destination_stop_id"`
	OriginDisplayName      string  `json:"origin_display_name"`
	DestinationDisplayName string  `json:"destination_display_name"`
	OriginDepartTime       string  `json:"origin_depart_time"`
	TripDate               string  `json:"trip_date"`
	SeatsAvailable         int     `json:"seats_available"`
	Price                  float64 `json:"price"`
	Currency               string  `json:"currency"`
	Status                 string  `json:"status"`
	TripStatus             string  `json:"trip_status"`
	PackageName            string  `json:"package_name,omitempty"`
}
