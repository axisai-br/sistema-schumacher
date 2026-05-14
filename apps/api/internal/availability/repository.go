package availability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Search(ctx context.Context, filter SearchFilter) ([]SearchResult, error) {
	query, args := buildAvailabilitySearchQuery(filter)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SearchResult, 0)
	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(
			&item.SegmentID,
			&item.TripID,
			&item.RouteID,
			&item.BoardStopID,
			&item.AlightStopID,
			&item.OriginStopID,
			&item.DestinationStopID,
			&item.OriginDisplayName,
			&item.DestinationDisplayName,
			&item.OriginDepartTime,
			&item.TripDate,
			&item.SeatsAvailable,
			&item.Price,
			&item.Currency,
			&item.Status,
			&item.TripStatus,
			&item.PackageName,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func buildAvailabilitySearchQuery(filter SearchFilter) (string, []interface{}) {
	limit := filter.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	tripStatusExpr := normalizedTripStatusSQL("t.status")

	query := `select
    concat(t.trip_id, ':', board.trip_stop_id, ':', alight.trip_stop_id) as segment_id,
    t.trip_id,
    t.route_id,
    board.trip_stop_id as board_stop_id,
    alight.trip_stop_id as alight_stop_id,
    board.stop_id as origin_stop_id,
    alight.stop_id as destination_stop_id,
    origin_stop.display_name as origin_display_name,
    destination_stop.display_name as destination_display_name,
    coalesce(to_char(board.depart_time::time, 'HH24:MI'), '') as origin_depart_time,
    to_char(t.trip_date, 'YYYY-MM-DD') as trip_date,
    greatest(coalesce(t.seats_available, 0), 0) as seats_available,
    rsp.price,
    'BRL' as currency,
    case
      when upper(coalesce(rsp.status, 'ACTIVE')) = 'INACTIVE' then 'INACTIVE'
      else 'ACTIVE'
    end as status,
    ` + tripStatusExpr + ` as trip_status,
    coalesce(t.package_name, '') as package_name
  from trips t
  join trip_stops board on board.trip_id = t.trip_id and board.is_active = true
  join trip_stops alight on alight.trip_id = t.trip_id and alight.is_active = true and alight.stop_sequence > board.stop_sequence
  join stops origin_stop on origin_stop.stop_id = board.stop_id
  join stops destination_stop on destination_stop.stop_id = alight.stop_id
  join route_segment_prices rsp
    on rsp.route_id = t.route_id
   and rsp.origin_stop_id = board.stop_id
   and rsp.destination_stop_id = alight.stop_id`

	args := []interface{}{}
	clauses := []string{
		activeTripStatusClause("t.status"),
	}

	if !filter.IncludePast {
		clauses = append(clauses, "t.trip_date >= current_date")
	}
	if filter.OnlyActive {
		clauses = append(clauses, "upper(coalesce(rsp.status, 'ACTIVE')) = 'ACTIVE'")
	}
	if filter.Origin != "" {
		args = append(args, normalizeSearchText(filter.Origin))
		clauses = append(clauses, fmt.Sprintf("%s = $%d", normalizedSearchColumnSQL("origin_stop.display_name"), len(args)))
	}
	if filter.Destination != "" {
		args = append(args, normalizeSearchText(filter.Destination))
		clauses = append(clauses, fmt.Sprintf("%s = $%d", normalizedSearchColumnSQL("destination_stop.display_name"), len(args)))
	}
	if filter.TripDate != nil {
		args = append(args, filter.TripDate.Format("2006-01-02"))
		clauses = append(clauses, fmt.Sprintf("t.trip_date = $%d::date", len(args)))
	}
	if filter.PackageName != "" {
		args = append(args, normalizeSearchText(filter.PackageName))
		clauses = append(clauses, fmt.Sprintf("%s = $%d", normalizedSearchColumnSQL("t.package_name"), len(args)))
	}
	if filter.Qty > 0 {
		args = append(args, filter.Qty)
		clauses = append(clauses, fmt.Sprintf("greatest(coalesce(t.seats_available, 0), 0) >= $%d", len(args)))
	}

	if len(clauses) > 0 {
		query += " where " + strings.Join(clauses, " and ")
	}

	args = append(args, limit)
	query += fmt.Sprintf(`
  order by t.trip_date asc, board.depart_time asc, origin_stop.display_name asc, destination_stop.display_name asc
  limit $%d`, len(args))

	return query, args
}

func activeTripStatusClause(column string) string {
	return fmt.Sprintf("upper(coalesce(%s, '')) in ('SCHEDULED', 'IN_PROGRESS', 'ATIVO', 'ACTIVE')", column)
}

func normalizedTripStatusSQL(column string) string {
	return fmt.Sprintf(`case
      when upper(coalesce(%s, '')) in ('ATIVO', 'ACTIVE') then 'SCHEDULED'
      else upper(coalesce(%s, ''))
    end`, column, column)
}

func normalizedSearchColumnSQL(column string) string {
	return fmt.Sprintf(
		"translate(lower(coalesce(%s, '')), 'áàâãäéèêëíìîïóòôõöúùûüçñ', 'aaaaaeeeeiiiiooooouuuucn')",
		column,
	)
}

func normalizeSearchText(value string) string {
	replacer := strings.NewReplacer(
		"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"Ç", "C", "ç", "c",
		"Ñ", "N", "ñ", "n",
	)
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, " /", "/")
	normalized = strings.ReplaceAll(normalized, "/ ", "/")
	return strings.ToLower(replacer.Replace(normalized))
}
