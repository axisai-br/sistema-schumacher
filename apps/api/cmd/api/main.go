package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"schumacher-tur/api/internal/advance_returns"
	"schumacher-tur/api/internal/affiliate"
	"schumacher-tur/api/internal/auth"
	"schumacher-tur/api/internal/automation"
	"schumacher-tur/api/internal/availability"
	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/buses"
	"schumacher-tur/api/internal/chat"
	"schumacher-tur/api/internal/driver_cards"
	"schumacher-tur/api/internal/drivers"
	"schumacher-tur/api/internal/fiscal_documents"
	"schumacher-tur/api/internal/imports_xlsx"
	"schumacher-tur/api/internal/invoices"
	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/pricing"
	"schumacher-tur/api/internal/products"
	"schumacher-tur/api/internal/purchase_orders"
	"schumacher-tur/api/internal/reports"
	"schumacher-tur/api/internal/routes"
	"schumacher-tur/api/internal/service_orders"
	"schumacher-tur/api/internal/shared/config"
	"schumacher-tur/api/internal/shared/db"
	httpx "schumacher-tur/api/internal/shared/http"
	"schumacher-tur/api/internal/suppliers"
	"schumacher-tur/api/internal/trip_advances"
	"schumacher-tur/api/internal/trip_expenses"
	"schumacher-tur/api/internal/trip_operations"
	"schumacher-tur/api/internal/trip_settlements"
	"schumacher-tur/api/internal/trip_validations"
	"schumacher-tur/api/internal/trips"
	"schumacher-tur/api/internal/users"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: .env file not found")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db error: %v", err)
	}
	defer pool.Close()

	authMiddleware, err := auth.NewAuthenticator(auth.Config{
		JWKSURL:            cfg.SupabaseJWKSURL,
		JWTSecret:          cfg.SupabaseJWTSecret,
		Issuer:             cfg.SupabaseIssuer,
		Audience:           cfg.SupabaseAudience,
		AllowMissingIssuer: cfg.SupabaseAllowMissingIssuer,
		ServiceTokens:      cfg.APIServiceTokens,
		Skip:               cfg.AuthDisabled,
	})
	if err != nil {
		log.Fatalf("auth error: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(ctx); err != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not ready", err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	paymentsRepo := payments.NewRepository(pool)
	paymentsSvc := payments.NewService(paymentsRepo, cfg)
	paymentsHandler := payments.NewHandler(paymentsSvc, cfg.PagarmeSecretKey)
	paymentsHandler.RegisterWebhooks(r)
	affiliateSvc := affiliate.NewService(affiliate.NewRepository(pool), cfg)
	affiliateHandler := affiliate.NewHandler(affiliateSvc)
	affiliateHandler.RegisterWebhooks(r)
	availabilitySvc := availability.NewService(availability.NewRepository(pool))
	pricingSvc := pricing.NewService(pricing.NewRepository(pool))
	bookingsSvc := bookings.NewService(bookings.NewRepository(pool), pricingSvc, paymentsSvc)
	reportsSvc := reports.NewService(reports.NewRepository(pool))
	evolutionSender := automation.NewEvolutionSender(cfg)
	openAIRunner := chat.NewOpenAIRunner(cfg)
	availabilityTool := chat.NewAvailabilityTool(availabilitySvc)
	pricingQuoteTool := chat.NewPricingQuoteTool(pricingSvc)
	bookingLookupTool := chat.NewBookingLookupTool(bookingsSvc)
	bookingCreateTool := chat.NewBookingCreateTool(bookingsSvc)
	bookingCancelTool := chat.NewBookingCancelTool(bookingsSvc)
	rescheduleAssistTool := chat.NewRescheduleAssistTool(bookingLookupTool, reportsSvc, availabilityTool)
	paymentStatusTool := chat.NewPaymentStatusTool(paymentsSvc)
	paymentCreateTool := chat.NewPaymentCreateTool(bookingsSvc, paymentsSvc)
	chatSvc := chat.NewService(chat.NewRepository(pool), cfg, log.Default(), evolutionSender, openAIRunner, availabilityTool, pricingQuoteTool, bookingLookupTool, bookingCreateTool, bookingCancelTool, rescheduleAssistTool, paymentStatusTool, paymentCreateTool)
	chatHandler := chat.NewHandler(chatSvc)
	automationSvc := automation.NewService(automation.NewRepository(pool), chatSvc, cfg, paymentsRepo, bookingsSvc)
	automation.StartChatBufferFlushLoop(ctx, automationSvc, cfg, log.Default())
	automation.StartChatAutoSendRetryLoop(ctx, automationSvc, cfg, log.Default())
	automationHandler := automation.NewHandler(automationSvc)
	automationHandler.RegisterWebhooks(r)

	r.Group(func(pr chi.Router) {
		pr.Use(authMiddleware.Middleware)

		routesHandler := routes.NewHandler(routes.NewService(routes.NewRepository(pool)))
		busesHandler := buses.NewHandler(buses.NewService(buses.NewRepository(pool)))
		driversHandler := drivers.NewHandler(drivers.NewService(drivers.NewRepository(pool)))
		tripsHandler := trips.NewHandler(trips.NewService(trips.NewRepository(pool)))
		tripOperationsHandler := trip_operations.NewHandler(trip_operations.NewService(trip_operations.NewRepository(pool)))
		availabilityHandler := availability.NewHandler(availabilitySvc)
		bookingsHandler := bookings.NewHandler(bookingsSvc)
		reportsHandler := reports.NewHandler(reportsSvc)
		pricingHandler := pricing.NewHandler(pricingSvc)

		routesHandler.RegisterRoutes(pr)
		busesHandler.RegisterRoutes(pr)
		driversHandler.RegisterRoutes(pr)
		tripsHandler.RegisterRoutes(pr)
		tripOperationsHandler.RegisterRoutes(pr)
		availabilityHandler.RegisterRoutes(pr)
		bookingsHandler.RegisterRoutes(pr)
		paymentsHandler.RegisterRoutes(pr)
		affiliateHandler.RegisterRoutes(pr)
		chatHandler.RegisterRoutes(pr)
		automationHandler.RegisterRoutes(pr)
		reportsHandler.RegisterRoutes(pr)
		pricingHandler.RegisterRoutes(pr)

		advancesRepo := trip_advances.NewRepository(pool)
		advancesHandler := trip_advances.NewHandler(trip_advances.NewService(advancesRepo))
		advancesHandler.RegisterRoutes(pr)

		expensesRepo := trip_expenses.NewRepository(pool)
		expensesHandler := trip_expenses.NewHandler(trip_expenses.NewService(expensesRepo))
		expensesHandler.RegisterRoutes(pr)

		settlementsRepo := trip_settlements.NewRepository(pool)
		settlementsHandler := trip_settlements.NewHandler(trip_settlements.NewService(settlementsRepo, advancesRepo, expensesRepo))
		settlementsHandler.RegisterRoutes(pr)

		cardsHandler := driver_cards.NewHandler(driver_cards.NewService(driver_cards.NewRepository(pool)))
		cardsHandler.RegisterRoutes(pr)

		validationsHandler := trip_validations.NewHandler(trip_validations.NewService(trip_validations.NewRepository(pool)))
		validationsHandler.RegisterRoutes(pr)

		returnsHandler := advance_returns.NewHandler(advance_returns.NewService(advance_returns.NewRepository(pool)))
		returnsHandler.RegisterRoutes(pr)

		fiscalHandler := fiscal_documents.NewHandler(fiscal_documents.NewService(fiscal_documents.NewRepository(pool)))
		fiscalHandler.RegisterRoutes(pr)

		// Almoxarifado e Compras
		suppliersHandler := suppliers.NewHandler(suppliers.NewService(suppliers.NewRepository(pool)))
		suppliersHandler.RegisterRoutes(pr)

		productsHandler := products.NewHandler(products.NewService(products.NewRepository(pool)))
		productsHandler.RegisterRoutes(pr)

		serviceOrdersHandler := service_orders.NewHandler(service_orders.NewService(service_orders.NewRepository(pool)))
		serviceOrdersHandler.RegisterRoutes(pr)

		purchaseOrdersHandler := purchase_orders.NewHandler(purchase_orders.NewService(purchase_orders.NewRepository(pool)))
		purchaseOrdersHandler.RegisterRoutes(pr)

		invoicesHandler := invoices.NewHandler(invoices.NewService(invoices.NewRepository(pool)))
		invoicesHandler.RegisterRoutes(pr)

		importsXLSXHandler := imports_xlsx.NewHandler(imports_xlsx.NewService(imports_xlsx.NewRepository(pool)))
		importsXLSXHandler.RegisterRoutes(pr)

		users.NewHandler(pool, cfg).RegisterRoutes(pr)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("api listening on %s", addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	allowAll := false
	origins := []string{}
	if cfg.CORSOrigins != "" {
		for _, o := range strings.Split(cfg.CORSOrigins, ",") {
			trimmed := strings.TrimSpace(o)
			if trimmed == "" {
				continue
			}
			if trimmed == "*" {
				allowAll = true
			} else {
				origins = append(origins, trimmed)
			}
		}
	}
	if cfg.CORSOrigins == "" && cfg.AppEnv != "production" {
		allowAll = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowAll || originAllowed(origin, origins) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, item := range allowed {
		if item == origin {
			return true
		}
	}
	return false
}
