package chat

import (
	"context"
	"schumacher-tur/api/internal/availability"
)

type AvailabilityTool struct {
	svc interface {
		Search(ctx context.Context, filter availability.SearchFilter) ([]availability.SearchResult, error)
	}
}

func NewAvailabilityTool(svc interface {
	Search(ctx context.Context, filter availability.SearchFilter) ([]availability.SearchResult, error)
},
) *AvailabilityTool {
	return &AvailabilityTool{svc: svc}
}

func (t *AvailabilityTool) Enabled() bool {
	return t != nil && t.svc != nil
}

func (t *AvailabilityTool) Search(ctx context.Context, input AvailabilitySearchInput) (AvailabilitySearchResult, error) {
	if !t.Enabled() {
		return AvailabilitySearchResult{}, ErrAgentToolNotConfigured
	}

	input = enrichAvailabilitySearchInput(input)
	filter := availability.SearchFilter{
		Origin:            input.Origin,
		Destination:       input.Destination,
		OriginStopID:      input.OriginStopID,
		DestinationStopID: input.DestinationStopID,
		RouteID:           input.RouteID,
		PackageName:       input.PackageName,
		TripDate:          input.TripDate,
		DateFrom:          input.DateFrom,
		DateTo:            input.DateTo,
		Qty:               input.Qty,
		Limit:             input.Limit,
		OnlyActive:        true,
		IncludePast:       false,
	}
	items, err := t.svc.Search(ctx, filter)
	if err != nil {
		return AvailabilitySearchResult{}, err
	}

	result := AvailabilitySearchResult{
		Filter:  input,
		Results: make([]AvailabilitySearchItem, 0, len(items)),
	}
	for _, item := range items {
		result.Results = append(result.Results, AvailabilitySearchItem{
			SegmentID:              item.SegmentID,
			TripID:                 item.TripID,
			RouteID:                item.RouteID,
			BoardStopID:            item.BoardStopID,
			AlightStopID:           item.AlightStopID,
			OriginStopID:           item.OriginStopID,
			DestinationStopID:      item.DestinationStopID,
			OriginDisplayName:      item.OriginDisplayName,
			DestinationDisplayName: item.DestinationDisplayName,
			OriginDepartTime:       item.OriginDepartTime,
			TripDate:               item.TripDate,
			SeatsAvailable:         item.SeatsAvailable,
			Price:                  item.Price,
			Currency:               item.Currency,
			Status:                 item.Status,
			TripStatus:             item.TripStatus,
			PackageName:            item.PackageName,
		})
	}

	return result, nil
}
