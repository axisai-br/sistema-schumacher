package trips

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnsupported = errors.New("operation not supported in production ticketing schema")
var ErrTripNotFound = errors.New("trip not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) List(ctx context.Context, filter ListFilter) ([]Trip, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := `select
    t.trip_id as id,
    t.route_id,
    coalesce(t.bus_id, '') as bus_id,
    null::text as driver_id,
    null::text as fare_id,
    null::text as request_id,
    (t.trip_date::timestamp) as departure_at,
    null::timestamp as arrival_at,
    t.status,
    t.status as operational_status,
    greatest(coalesce(t.seats_total, 0), 0) as seats_total,
    greatest(coalesce(t.seats_available, 0), 0) as seats_available,
    0::numeric as estimated_km,
    null::timestamp as dispatch_validated_at,
    null::text as dispatch_validated_by,
    null::text as pair_trip_id,
    null::text as notes,
    t.created_at,
    t.created_at as updated_at
    from trips t`
	args := []interface{}{}
	clauses := []string{}

	if filter.Status != "" && filter.Status != "ALL" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("t.status = $%d", len(args)))
	}

	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		clauses = append(clauses, fmt.Sprintf(`(
      t.trip_id ilike $%d
      or t.route_id ilike $%d
      or coalesce(t.bus_id, '') ilike $%d
      or t.status ilike $%d
    )`, len(args), len(args), len(args), len(args)))
	}

	if len(clauses) > 0 {
		query += " where " + strings.Join(clauses, " and ")
	}

	query += " order by t.trip_date desc, t.created_at desc"
	args = append(args, limit)
	query += fmt.Sprintf(" limit $%d", len(args))
	args = append(args, offset)
	query += fmt.Sprintf(" offset $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Trip{}
	for rows.Next() {
		var item Trip
		if err := scanTrip(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Trip, error) {
	var item Trip
	row := r.pool.QueryRow(ctx, `select
    t.trip_id as id,
    t.route_id,
    coalesce(t.bus_id, '') as bus_id,
    null::text as driver_id,
    null::text as fare_id,
    null::text as request_id,
    (t.trip_date::timestamp) as departure_at,
    null::timestamp as arrival_at,
    t.status,
    t.status as operational_status,
    greatest(coalesce(t.seats_total, 0), 0) as seats_total,
    greatest(coalesce(t.seats_available, 0), 0) as seats_available,
    0::numeric as estimated_km,
    null::timestamp as dispatch_validated_at,
    null::text as dispatch_validated_by,
    null::text as pair_trip_id,
    null::text as notes,
    t.created_at,
    t.created_at as updated_at
    from trips t where t.trip_id=$1`, id)
	if err := scanTrip(row, &item); err != nil {
		return item, err
	}
	return item, nil
}

func (r *Repository) Create(ctx context.Context, input CreateTripInput) (Trip, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Trip{}, err
	}
	defer tx.Rollback(ctx)

	routeID := strings.TrimSpace(input.RouteID)
	busID := strings.TrimSpace(input.BusID)
	if routeID == "" || busID == "" || input.DepartureAt.IsZero() {
		return Trip{}, errors.New("route_id, bus_id and departure_at are required")
	}

	if err := r.ensureRouteExistsTx(ctx, tx, routeID); err != nil {
		return Trip{}, err
	}

	tripDateSource := input.DepartureAt
	if len(input.Stops) > 0 {
		firstStopAt, err := earliestStopDateTime(input.Stops)
		if err != nil {
			return Trip{}, err
		}
		tripDateSource = firstStopAt
	}
	if tripDateSource.IsZero() {
		return Trip{}, errors.New("departure_at is required")
	}

	tripDate := tripDateSource.In(time.UTC).Format("2006-01-02")
	tripID, err := r.allocateTripIDTx(ctx, tx, routeID, tripDate)
	if err != nil {
		return Trip{}, err
	}

	seatsTotal, err := r.resolveRouteSeatCapacityTx(ctx, tx, routeID)
	if err != nil {
		return Trip{}, err
	}
	if seatsTotal < 0 {
		seatsTotal = 0
	}

	status := "ATIVO"
	if input.Status != nil {
		normalized := strings.ToUpper(strings.TrimSpace(*input.Status))
		if normalized != "" {
			status = normalized
		}
	}

	if _, err := tx.Exec(ctx, `
    insert into trips (
      trip_id,
      route_id,
      trip_date,
      bus_id,
      default_price,
      seats_total,
      seats_available,
      duration_hours,
      status,
      package_name,
      created_at
    ) values ($1, $2, $3::date, $4, 0, $5, $5, null, $6, null, now())
  `, tripID, routeID, tripDate, busID, seatsTotal, status); err != nil {
		return Trip{}, err
	}

	routeStops, err := r.listRouteTemplateStopsTx(ctx, tx, routeID)
	if err != nil {
		return Trip{}, err
	}
	if len(routeStops) < 2 {
		return Trip{}, errors.New("route must have at least 2 active stops")
	}

	customDepartByRouteStopID := make(map[string]*time.Time, len(input.Stops))
	if len(input.Stops) > 0 {
		if len(input.Stops) != len(routeStops) {
			return Trip{}, errors.New("stops schedule must include all route stops")
		}
		for _, stop := range input.Stops {
			routeStopID := strings.TrimSpace(stop.RouteStopID)
			if routeStopID == "" {
				return Trip{}, errors.New("stops schedule contains empty route_stop_id")
			}
			if _, exists := customDepartByRouteStopID[routeStopID]; exists {
				return Trip{}, errors.New("duplicate route_stop_id in stops schedule")
			}
			customDepartByRouteStopID[routeStopID] = stop.DepartAt
		}
	}

	for _, routeStop := range routeStops {
		departAt := routeStop.DepartAt
		if len(customDepartByRouteStopID) > 0 {
			customDepartAt, ok := customDepartByRouteStopID[routeStop.RouteStopID]
			if !ok {
				return Trip{}, errors.New("stops schedule is missing route stop: " + routeStop.RouteStopID)
			}
			departAt = customDepartAt
		}
		if err := r.insertTripStopTx(ctx, tx, tripID, routeStop.StopID, routeStop.StopOrder, departAt); err != nil {
			return Trip{}, err
		}
	}

	if err := r.refreshAvailableSegmentsForRouteTx(ctx, tx, routeID); err != nil {
		return Trip{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Trip{}, err
	}

	return r.Get(ctx, tripID)
}

func (r *Repository) Update(ctx context.Context, id string, input UpdateTripInput) (Trip, error) {
	return Trip{}, ErrUnsupported
}

func (r *Repository) ListSeats(ctx context.Context, tripID string, boardStopID *string, alightStopID *string) ([]TripSeat, error) {
	rows, err := r.pool.Query(ctx, `
    with trip_capacity as (
      select greatest(coalesce(seats_total, 0), 0) as seats_total
      from trips
      where trip_id = $1
      limit 1
    )
    select
      gs::text as id,
      gs as seat_number,
      true as is_active,
      exists (
        select 1
        from passengers p
        join bookings b on b.booking_id = p.booking_id
        where p.trip_id = $1
          and p.seat_number = gs::text
          and upper(coalesce(b.status, '')) not in ('CANCELLED', 'EXPIRED')
      ) as is_taken
    from trip_capacity tc
    cross join generate_series(1, coalesce(nullif(tc.seats_total, 0), 50)) as gs
    order by gs asc
  `, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seats := []TripSeat{}
	for rows.Next() {
		var seat TripSeat
		if err := rows.Scan(&seat.ID, &seat.SeatNumber, &seat.IsActive, &seat.IsTaken); err != nil {
			return nil, err
		}
		seats = append(seats, seat)
	}
	return seats, rows.Err()
}

func (r *Repository) ListStops(ctx context.Context, tripID string) ([]TripStop, error) {
	rows, err := r.pool.Query(ctx, `
    select
      ts.trip_stop_id as id,
      ts.trip_id,
      ts.stop_id as route_stop_id,
      s.display_name as city,
      ts.stop_sequence as stop_order,
      0::numeric as leg_distance_km,
      0::numeric as cumulative_distance_km,
      null::timestamp as arrive_at,
      case
        when ts.depart_time is null then null::timestamp
        else (t.trip_date::timestamp + ts.depart_time)
      end as depart_at,
      ts.created_at,
      ts.created_at as updated_at
    from trip_stops ts
    join trips t on t.trip_id = ts.trip_id
    join stops s on s.stop_id = ts.stop_id
    where ts.trip_id = $1
      and ts.is_active = true
    order by ts.stop_sequence asc
  `, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TripStop{}
	for rows.Next() {
		var item TripStop
		if err := rowscanTripStop(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetDetails(ctx context.Context, tripID string) (TripDetails, error) {
	trip, err := r.Get(ctx, tripID)
	if err != nil {
		return TripDetails{}, err
	}

	rows, err := r.pool.Query(ctx, `
    with passenger_base as (
      select
        p.passenger_id,
        p.booking_id,
        p.full_name,
        coalesce(p.document, '') as document,
        coalesce(p.document_type, '') as document_type,
        coalesce(p.cpf, '') as cpf,
        coalesce(p.rg, '') as rg,
        coalesce(p.cnh, '') as cnh,
        coalesce(to_char(p.birth_date, 'YYYY-MM-DD'), '') as birth_date,
        coalesce(p.birth_certificate_number, '') as birth_certificate_number,
        coalesce(p.birth_city, '') as birth_city,
        coalesce(p.phone, '') as phone,
        coalesce(p.seat_number, '') as seat_number,
        coalesce(p.origin_stop_id, b.origin_stop_id, '') as origin_stop_id,
        coalesce(origin_stop.display_name, p.origin_stop_id, b.origin_stop_id, '') as origin_name,
        coalesce(p.destination_stop_id, b.destination_stop_id, '') as destination_stop_id,
        coalesce(destination_stop.display_name, p.destination_stop_id, b.destination_stop_id, '') as destination_name,
        upper(coalesce(b.status, 'PENDING')) as booking_status,
        upper(coalesce(pay.payment_status, b.status::text, 'PENDING')) as payment_status,
        case when position('CRIANCA_DE_COLO_ATE_5_ANOS' in upper(coalesce(p.notes, ''))) > 0 then true else false end as is_lap_child,
        coalesce(pay.amount_total, 0)::numeric as booking_total_amount,
        coalesce(pay.amount_paid, 0)::numeric as booking_paid_amount,
        coalesce(pay.amount_due, greatest(coalesce(pay.amount_total, 0) - coalesce(pay.amount_paid, 0), 0))::numeric as booking_due_amount,
        greatest(
          count(*) filter (
            where position('CRIANCA_DE_COLO_ATE_5_ANOS' in upper(coalesce(p.notes, ''))) = 0
          ) over (partition by p.booking_id),
          1
        )::numeric as chargeable_passengers
      from passengers p
      join bookings b on b.booking_id = p.booking_id
      left join booking_payment_details pay on pay.booking_id = b.booking_id
      left join stops origin_stop on origin_stop.stop_id = coalesce(p.origin_stop_id, b.origin_stop_id)
      left join stops destination_stop on destination_stop.stop_id = coalesce(p.destination_stop_id, b.destination_stop_id)
      where p.trip_id = $1
        and upper(coalesce(b.status, '')) not in ('CANCELLED', 'EXPIRED')
    )
    select
      passenger_id,
      booking_id,
      full_name,
      document,
      document_type,
      cpf,
      rg,
      cnh,
      birth_date,
      birth_certificate_number,
      birth_city,
      phone,
      seat_number,
      origin_stop_id,
      origin_name,
      destination_stop_id,
      destination_name,
      booking_status,
      payment_status,
      is_lap_child,
      case
        when is_lap_child then 0::numeric
        else booking_total_amount / chargeable_passengers
      end as passenger_total_amount,
      case
        when is_lap_child then 0::numeric
        else booking_paid_amount / chargeable_passengers
      end as passenger_paid_amount,
      case
        when is_lap_child then 0::numeric
        else booking_due_amount / chargeable_passengers
      end as passenger_due_amount
    from passenger_base
    order by origin_name asc, destination_name asc, full_name asc
  `, tripID)
	if err != nil {
		return TripDetails{}, err
	}
	defer rows.Close()

	passengers := make([]TripDetailsPassenger, 0)
	totals := TripDetailsTotals{}

	type segmentAccumulator struct {
		OriginStopID      string
		OriginName        string
		DestinationStopID string
		DestinationName   string
		PassengersCount   int
		TotalAmount       float64
		PaidAmount        float64
		DueAmount         float64
	}

	segments := make(map[string]*segmentAccumulator)

	for rows.Next() {
		var item TripDetailsPassenger
		if err := rows.Scan(
			&item.PassengerID,
			&item.BookingID,
			&item.Name,
			&item.Document,
			&item.DocumentType,
			&item.CPF,
			&item.RG,
			&item.CNH,
			&item.BirthDate,
			&item.BirthCertificateNumber,
			&item.BirthCity,
			&item.Phone,
			&item.SeatNumber,
			&item.OriginStopID,
			&item.OriginName,
			&item.DestinationStopID,
			&item.DestinationName,
			&item.BookingStatus,
			&item.PaymentStatus,
			&item.IsLapChild,
			&item.TotalAmount,
			&item.PaidAmount,
			&item.DueAmount,
		); err != nil {
			return TripDetails{}, err
		}

		passengers = append(passengers, item)
		totals.PassengersCount++
		totals.TotalAmount += item.TotalAmount
		totals.PaidAmount += item.PaidAmount
		totals.DueAmount += item.DueAmount

		segmentKey := item.OriginStopID + "->" + item.DestinationStopID
		acc := segments[segmentKey]
		if acc == nil {
			acc = &segmentAccumulator{
				OriginStopID:      item.OriginStopID,
				OriginName:        item.OriginName,
				DestinationStopID: item.DestinationStopID,
				DestinationName:   item.DestinationName,
			}
			segments[segmentKey] = acc
		}
		acc.PassengersCount++
		acc.TotalAmount += item.TotalAmount
		acc.PaidAmount += item.PaidAmount
		acc.DueAmount += item.DueAmount
	}
	if err := rows.Err(); err != nil {
		return TripDetails{}, err
	}

	segmentItems := make([]TripDetailsSegmentSummary, 0, len(segments))
	for _, acc := range segments {
		segmentItems = append(segmentItems, TripDetailsSegmentSummary{
			OriginStopID:      acc.OriginStopID,
			OriginName:        acc.OriginName,
			DestinationStopID: acc.DestinationStopID,
			DestinationName:   acc.DestinationName,
			PassengersCount:   acc.PassengersCount,
			TotalAmount:       acc.TotalAmount,
			PaidAmount:        acc.PaidAmount,
			DueAmount:         acc.DueAmount,
		})
	}
	sort.Slice(segmentItems, func(i, j int) bool {
		if segmentItems[i].OriginName == segmentItems[j].OriginName {
			return segmentItems[i].DestinationName < segmentItems[j].DestinationName
		}
		return segmentItems[i].OriginName < segmentItems[j].OriginName
	})

	return TripDetails{
		Trip:       trip,
		Totals:     totals,
		Segments:   segmentItems,
		Passengers: passengers,
	}, nil
}

func (r *Repository) CreateStop(ctx context.Context, tripID string, input CreateTripStopInput) (TripStop, error) {
	return TripStop{}, ErrUnsupported
}

func (r *Repository) ListSegmentPrices(ctx context.Context, tripID string) (TripSegmentPriceMatrix, error) {
	routeID, err := r.getTripRouteID(ctx, tripID)
	if err != nil {
		return TripSegmentPriceMatrix{}, err
	}

	stops, err := r.listTripOrderedStops(ctx, tripID)
	if err != nil {
		return TripSegmentPriceMatrix{}, err
	}

	rows, err := r.pool.Query(ctx, `
    with ordered_stops as (
      select
        ts.stop_id,
        s.display_name,
        ts.stop_sequence
      from trip_stops ts
      join stops s on s.stop_id = ts.stop_id
      where ts.trip_id = $1
        and ts.is_active = true
      order by ts.stop_sequence asc
    )
    select
      o.stop_id as origin_stop_id,
      o.display_name as origin_display_name,
      o.stop_sequence as origin_stop_order,
      d.stop_id as destination_stop_id,
      d.display_name as destination_display_name,
      d.stop_sequence as destination_stop_order,
      rsp.price,
      rsp.status,
      rsp.created_at,
      rsp.updated_at,
      (rsp.route_id is not null) as configured
    from ordered_stops o
    join ordered_stops d on d.stop_sequence > o.stop_sequence
    left join route_segment_prices rsp
      on rsp.route_id = $2
     and rsp.origin_stop_id = o.stop_id
     and rsp.destination_stop_id = d.stop_id
    order by o.stop_sequence asc, d.stop_sequence asc
  `, tripID, routeID)
	if err != nil {
		return TripSegmentPriceMatrix{}, err
	}
	defer rows.Close()

	items := make([]TripSegmentPriceItem, 0)
	for rows.Next() {
		var item TripSegmentPriceItem
		var status *string
		var createdAt *time.Time
		var updatedAt *time.Time
		if err := rows.Scan(
			&item.OriginStopID,
			&item.OriginDisplayName,
			&item.OriginStopOrder,
			&item.DestinationStopID,
			&item.DestinationDisplayName,
			&item.DestinationStopOrder,
			&item.Price,
			&status,
			&createdAt,
			&updatedAt,
			&item.Configured,
		); err != nil {
			return TripSegmentPriceMatrix{}, err
		}

		if status == nil || !item.Configured {
			item.Status = "MISSING"
		} else {
			item.Status = strings.ToUpper(strings.TrimSpace(*status))
		}
		item.CreatedAt = createdAt
		item.UpdatedAt = updatedAt
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return TripSegmentPriceMatrix{}, err
	}

	return TripSegmentPriceMatrix{
		TripID:  tripID,
		RouteID: routeID,
		Stops:   stops,
		Items:   items,
	}, nil
}

func (r *Repository) UpsertSegmentPrices(ctx context.Context, tripID string, input UpsertTripSegmentPricesInput) (TripSegmentPriceMatrix, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TripSegmentPriceMatrix{}, err
	}
	defer tx.Rollback(ctx)

	routeID, err := r.getTripRouteIDTx(ctx, tx, tripID)
	if err != nil {
		return TripSegmentPriceMatrix{}, err
	}

	stopOrderByStopID, err := r.getTripStopOrderMapTx(ctx, tx, tripID)
	if err != nil {
		return TripSegmentPriceMatrix{}, err
	}

	type normalizedSegmentUpdate struct {
		OriginStopID      string
		DestinationStopID string
		Price             *float64
		Status            string
	}

	updates := make([]normalizedSegmentUpdate, 0, len(input.Items))
	seen := make(map[string]int, len(input.Items))

	for _, item := range input.Items {
		origin := strings.TrimSpace(item.OriginStopID)
		destination := strings.TrimSpace(item.DestinationStopID)
		originOrder, originOk := stopOrderByStopID[origin]
		destinationOrder, destinationOk := stopOrderByStopID[destination]
		if !originOk || !destinationOk || originOrder >= destinationOrder {
			return TripSegmentPriceMatrix{}, ErrInvalidSegmentPair
		}

		status := "ACTIVE"
		if item.Status != nil {
			status = strings.ToUpper(strings.TrimSpace(*item.Status))
		}
		if status == "" {
			status = "ACTIVE"
		}

		key := origin + "->" + destination
		normalized := normalizedSegmentUpdate{
			OriginStopID:      origin,
			DestinationStopID: destination,
			Price:             item.Price,
			Status:            status,
		}

		if idx, ok := seen[key]; ok {
			updates[idx] = normalized
			continue
		}
		seen[key] = len(updates)
		updates = append(updates, normalized)
	}

	for _, item := range updates {
		if item.Price == nil {
			if _, err := tx.Exec(ctx, `
        delete from route_segment_prices
        where route_id = $1
          and origin_stop_id = $2
          and destination_stop_id = $3
      `, routeID, item.OriginStopID, item.DestinationStopID); err != nil {
				return TripSegmentPriceMatrix{}, err
			}
			continue
		}

		if _, err := tx.Exec(ctx, `
      insert into route_segment_prices (
        route_id,
        origin_stop_id,
        destination_stop_id,
        price,
        status,
        created_at,
        updated_at
      ) values ($1, $2, $3, $4, $5, now(), now())
      on conflict (route_id, origin_stop_id, destination_stop_id)
      do update set
        price = excluded.price,
        status = excluded.status,
        updated_at = now()
    `, routeID, item.OriginStopID, item.DestinationStopID, *item.Price, item.Status); err != nil {
			return TripSegmentPriceMatrix{}, err
		}
	}

	if err := r.refreshAvailableSegmentsForRouteTx(ctx, tx, routeID); err != nil {
		return TripSegmentPriceMatrix{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return TripSegmentPriceMatrix{}, err
	}

	return r.ListSegmentPrices(ctx, tripID)
}

func (r *Repository) EnsureTripStopsFromRoute(ctx context.Context, tripID string) error {
	return nil
}

func (r *Repository) ValidateRouteReadiness(ctx context.Context, routeID string) ([]string, error) {
	trimmedRouteID := strings.TrimSpace(routeID)
	if trimmedRouteID == "" {
		return []string{"route_missing"}, nil
	}

	var routeExists bool
	var routeActive bool
	if err := r.pool.QueryRow(ctx, `
    select true, coalesce(is_active, false)
    from routes
    where route_id = $1
    limit 1
  `, trimmedRouteID).Scan(&routeExists, &routeActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}

	missing := make([]string, 0, 4)
	if !routeExists {
		missing = append(missing, "route_missing")
		return missing, nil
	}
	if !routeActive {
		missing = append(missing, "route_inactive")
	}

	var stopCount int
	if err := r.pool.QueryRow(ctx, `
    with source_trip as (
      select trip_id
      from trips
      where route_id = $1
      order by
        case when upper(coalesce(status, '')) = 'TEMPLATE' then 0 else 1 end,
        trip_date desc,
        created_at desc
      limit 1
    )
    select count(*)
    from source_trip st
    join trip_stops ts on ts.trip_id = st.trip_id
    where ts.is_active = true
  `, trimmedRouteID).Scan(&stopCount); err != nil {
		return nil, err
	}
	if stopCount < 2 {
		missing = append(missing, "route_stops_minimum")
	}

	var activeSegmentPrices int
	if err := r.pool.QueryRow(ctx, `
    select count(*)
    from route_segment_prices
    where route_id = $1
      and upper(coalesce(status, 'ACTIVE')) = 'ACTIVE'
  `, trimmedRouteID).Scan(&activeSegmentPrices); err != nil {
		return nil, err
	}
	if activeSegmentPrices == 0 {
		missing = append(missing, "segment_prices_missing")
	}

	return missing, nil
}

type routeTemplateStop struct {
	RouteStopID string
	StopID      string
	StopOrder   int
	DepartAt    *time.Time
}

func (r *Repository) listRouteTemplateStopsTx(ctx context.Context, tx pgx.Tx, routeID string) ([]routeTemplateStop, error) {
	rows, err := tx.Query(ctx, `
    with source_trip as (
      select trip_id
      from trips
      where route_id = $1
      order by trip_date desc, created_at desc
      limit 1
    )
    select
      ts.trip_stop_id,
      ts.stop_id,
      ts.stop_sequence,
      case
        when ts.depart_time is null then null::timestamp
        else (t.trip_date::timestamp + ts.depart_time)
      end as depart_at
    from source_trip st
    join trips t on t.trip_id = st.trip_id
    join trip_stops ts on ts.trip_id = st.trip_id
    where ts.is_active = true
    order by ts.stop_sequence asc
  `, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]routeTemplateStop, 0)
	for rows.Next() {
		var item routeTemplateStop
		if err := rows.Scan(&item.RouteStopID, &item.StopID, &item.StopOrder, &item.DepartAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) resolveRouteSeatCapacityTx(ctx context.Context, tx pgx.Tx, routeID string) (int, error) {
	var seatsTotal int
	if err := tx.QueryRow(ctx, `
    select coalesce(seats_total, 46)
    from trips
    where route_id = $1
      and upper(coalesce(status, '')) <> 'TEMPLATE'
    order by trip_date desc, created_at desc
    limit 1
  `, routeID).Scan(&seatsTotal); err == nil {
		return seatsTotal, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	return 46, nil
}

func (r *Repository) ensureRouteExistsTx(ctx context.Context, tx pgx.Tx, routeID string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `select exists(select 1 from routes where route_id = $1)`, routeID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) allocateTripIDTx(ctx context.Context, tx pgx.Tx, routeID string, tripDate string) (string, error) {
	base := strings.ToLower(strings.ReplaceAll(routeID, "_", "-")) + "-" + tripDate
	for attempt := 0; attempt < 100; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%d", base, attempt+1)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from trips where trip_id = $1)`, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", errors.New("could not allocate trip_id")
}

func (r *Repository) insertTripStopTx(ctx context.Context, tx pgx.Tx, tripID string, stopID string, stopOrder int, departAt *time.Time) error {
	for attempt := 0; attempt < 16; attempt++ {
		tripStopID := fmt.Sprintf("%s_%s_%d", tripID, strings.ToLower(stopID), time.Now().UnixNano()+int64(attempt))
		_, err := tx.Exec(ctx, `
      insert into trip_stops (
        trip_stop_id,
        trip_id,
        stop_id,
        stop_sequence,
        depart_time,
        is_active,
        created_at
      ) values ($1, $2, $3, $4, $5, true, now())
    `, tripStopID, tripID, stopID, stopOrder, departAt)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return err
	}
	return errors.New("could not insert trip stop after retries")
}

func earliestStopDateTime(stops []CreateTripScheduleStopInput) (time.Time, error) {
	var earliest time.Time
	for _, stop := range stops {
		if stop.DepartAt == nil {
			return time.Time{}, errors.New("depart_at is required for all stops")
		}
		if earliest.IsZero() || stop.DepartAt.Before(earliest) {
			earliest = *stop.DepartAt
		}
	}
	if earliest.IsZero() {
		return time.Time{}, errors.New("stops schedule must contain valid depart_at values")
	}
	return earliest, nil
}

func (r *Repository) getTripRouteID(ctx context.Context, tripID string) (string, error) {
	var routeID string
	if err := r.pool.QueryRow(ctx, `select route_id from trips where trip_id = $1`, tripID).Scan(&routeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrTripNotFound
		}
		return "", err
	}
	return routeID, nil
}

func (r *Repository) getTripRouteIDTx(ctx context.Context, tx pgx.Tx, tripID string) (string, error) {
	var routeID string
	if err := tx.QueryRow(ctx, `select route_id from trips where trip_id = $1`, tripID).Scan(&routeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrTripNotFound
		}
		return "", err
	}
	return routeID, nil
}

func (r *Repository) listTripOrderedStops(ctx context.Context, tripID string) ([]TripSegmentPriceStop, error) {
	rows, err := r.pool.Query(ctx, `
    select
      ts.stop_id,
      s.display_name,
      ts.stop_sequence
    from trip_stops ts
    join stops s on s.stop_id = ts.stop_id
    where ts.trip_id = $1
      and ts.is_active = true
    order by ts.stop_sequence asc
  `, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TripSegmentPriceStop, 0)
	for rows.Next() {
		var item TripSegmentPriceStop
		if err := rows.Scan(&item.StopID, &item.DisplayName, &item.StopOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) getTripStopOrderMapTx(ctx context.Context, tx pgx.Tx, tripID string) (map[string]int, error) {
	rows, err := tx.Query(ctx, `
    select stop_id, stop_sequence
    from trip_stops
    where trip_id = $1
      and is_active = true
  `, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var stopID string
		var stopOrder int
		if err := rows.Scan(&stopID, &stopOrder); err != nil {
			return nil, err
		}
		result[stopID] = stopOrder
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) refreshAvailableSegmentsForRouteTx(ctx context.Context, tx pgx.Tx, routeID string) error {
	_, err := tx.Exec(ctx, `select refresh_available_segments_for_route($1)`, routeID)
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42883" {
		// Function might not exist before migration rollout.
		return nil
	}
	return err
}

func scanTrip(scanner interface {
	Scan(dest ...interface{}) error
}, item *Trip) error {
	return scanner.Scan(
		&item.ID,
		&item.RouteID,
		&item.BusID,
		&item.DriverID,
		&item.FareID,
		&item.RequestID,
		&item.DepartureAt,
		&item.ArrivalAt,
		&item.Status,
		&item.OperationalStatus,
		&item.SeatsTotal,
		&item.SeatsAvailable,
		&item.EstimatedKM,
		&item.DispatchValidatedAt,
		&item.DispatchValidatedBy,
		&item.PairTripID,
		&item.Notes,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
}

func rowscanTripStop(scanner interface {
	Scan(dest ...interface{}) error
}, item *TripStop) error {
	return scanner.Scan(
		&item.ID,
		&item.TripID,
		&item.RouteStopID,
		&item.City,
		&item.StopOrder,
		&item.LegDistanceKM,
		&item.CumulativeDistanceKM,
		&item.ArriveAt,
		&item.DepartAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}
