package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"tempmail/config"
	"tempmail/handler"
	"tempmail/mail"
	"tempmail/middleware"
	"tempmail/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()
	log.Println("✓ Database connected")

	seedSettings(ctx, db, cfg)

	domainH := handler.NewDomainHandler(db, cfg.SMTPServerIP)
	router := buildRouter(db, domainH, cfg)
	delivery := mail.NewDeliveryService(db)
	smtpServer := mail.NewSMTPServer(mail.SMTPConfig{
		Addr:            cfg.SMTPAddr,
		Domain:          cfg.SMTPDomain,
		MaxMessageBytes: cfg.SMTPMaxMessage,
		ReadTimeout:     cfg.SMTPReadTimeout,
		WriteTimeout:    cfg.SMTPWriteTimeout,
		MaxRecipients:   cfg.SMTPMaxRecipients,
	}, delivery)

	httpServer := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go runMailboxCleaner(ctx, db)
	go runMXVerifier(ctx, db, domainH)
	go writeAdminKeyFile(ctx, db, cfg.AdminKeyFile)

	httpErrCh := make(chan error, 1)
	smtpErrCh := make(chan error, 1)
	go func() {
		log.Printf("✓ HTTP server listening on %s", cfg.HTTPAddr)
		httpErrCh <- httpServer.ListenAndServe()
	}()
	go func() {
		log.Printf("✓ SMTP server listening on %s", cfg.SMTPAddr)
		smtpErrCh <- smtpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutting down servers...")
	case err := <-httpErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	case err := <-smtpErrCh:
		if err != nil {
			log.Fatalf("smtp server error: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("http shutdown error: %v", err)
	}
	if err := smtpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("smtp shutdown error: %v", err)
	}
	log.Println("Server exited")
}

func seedSettings(ctx context.Context, db *store.Store, cfg *config.Config) {
	if cfg.SMTPServerIP == "" {
		return
	}
	if dbIP, _ := db.GetSetting(ctx, "smtp_server_ip"); dbIP != cfg.SMTPServerIP {
		if err := db.SetSetting(ctx, "smtp_server_ip", cfg.SMTPServerIP); err == nil {
			log.Printf("✓ Synced SMTP_SERVER_IP from env to DB: %s", cfg.SMTPServerIP)
		}
	}
}

func buildRouter(db *store.Store, domainH *handler.DomainHandler, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:  []string{"*"},
		AllowMethods:  []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders: []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
		MaxAge:        12 * time.Hour,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Unix()})
	})

	accountH := handler.NewAccountHandler(db)
	mailboxH := handler.NewMailboxHandler(db)
	emailH := handler.NewEmailHandler(db)
	retainedMailH := handler.NewRetainedMailHandler(db)
	settingH := handler.NewSettingHandler(db, domainH, "")
	registerH := handler.NewRegisterHandler(db)
	statsH := handler.NewStatsHandler(db)

	public := r.Group("/public")
	{
		public.GET("/settings", settingH.GetPublic)
		public.POST("/register", registerH.Register)
		public.GET("/stats", statsH.Get)
	}

	rl := middleware.NewInMemoryRateLimiter(cfg.RateLimit, cfg.RateWindow)
	api := r.Group("/api")
	api.Use(middleware.Auth(db))
	api.Use(middleware.RateLimit(rl))
	{
		api.GET("/me", accountH.Me)
		api.GET("/domains", domainH.List)
		api.GET("/domains/:id/status", domainH.GetStatus)
		api.GET("/stats", statsH.Get)
		api.POST("/domains/submit", domainH.Submit)
		api.POST("/mailboxes", mailboxH.Create)
		api.GET("/mailboxes", mailboxH.List)
		api.DELETE("/mailboxes/:id", mailboxH.Delete)
		api.PUT("/mailboxes/:id/renew", mailboxH.Renew)
		api.GET("/mailboxes/:id/emails", emailH.List)
		api.GET("/mailboxes/:id/emails/:email_id", emailH.Get)
		api.DELETE("/mailboxes/:id/emails/:email_id", emailH.Delete)

		admin := api.Group("/admin")
		admin.Use(middleware.AdminOnly())
		{
			admin.POST("/accounts", accountH.Create)
			admin.GET("/accounts", accountH.List)
			admin.DELETE("/accounts/:id", accountH.Delete)
			admin.POST("/domains", domainH.Add)
			admin.DELETE("/domains/:id", domainH.Delete)
			admin.PUT("/domains/:id/toggle", domainH.Toggle)
			admin.POST("/domains/mx-import", domainH.MXImport)
			admin.POST("/domains/mx-register", domainH.MXRegister)
			admin.POST("/domains/cf-create", domainH.CFCreate)
			admin.DELETE("/domains/:id/cf", domainH.CFDelete)
			admin.PUT("/domains/:id/hostname", domainH.UpdateHostname)
			admin.PUT("/domains/batch/toggle", domainH.BatchToggle)
			admin.PUT("/domains/batch/delete", domainH.BatchDelete)
			admin.PUT("/domains/batch/cf-delete", domainH.BatchCFDelete)
			admin.GET("/domains/:id/status", domainH.GetStatus)
			admin.GET("/settings", settingH.AdminGetAll)
			admin.PUT("/settings", settingH.AdminUpdate)
			admin.GET("/retained-mails", retainedMailH.List)
			admin.GET("/retained-mails/:id", retainedMailH.Get)
			admin.DELETE("/retained-mails/:id", retainedMailH.Delete)
		}
	}

	registerFrontendRoutes(r)
	return r
}

func runMailboxCleaner(ctx context.Context, db *store.Store) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	log.Println("✓ Mailbox expiry cleaner started (TTL=30min, interval=1min)")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if deleted, err := db.DeleteExpiredMailboxes(context.Background()); err != nil {
				log.Printf("[cleaner] error: %v", err)
			} else if deleted > 0 {
				log.Printf("[cleaner] deleted %d expired mailboxes", deleted)
			}
		}
	}
}

func runMXVerifier(ctx context.Context, db *store.Store, domainH *handler.DomainHandler) {
	pendingTicker := time.NewTicker(30 * time.Second)
	recheckTicker := time.NewTicker(6 * time.Hour)
	defer pendingTicker.Stop()
	defer recheckTicker.Stop()
	log.Println("✓ MX domain verifier started (pending check=30s, active re-check=6h)")
	for {
		select {
		case <-ctx.Done():
			return
		case <-pendingTicker.C:
			pendingDomains, err := db.ListPendingDomains(context.Background())
			if err != nil {
				log.Printf("[mx-verifier] list pending error: %v", err)
				continue
			}
			serverIP := domainH.GetServerIP()
			for _, d := range pendingDomains {
				matched, _, mxStatus := store.CheckDomainMX(d.Domain, serverIP)
				db.TouchDomainCheckTime(context.Background(), d.ID)
				if matched {
					if err := db.PromoteDomainToActive(context.Background(), d.ID); err != nil {
						log.Printf("[mx-verifier] promote %s error: %v", d.Domain, err)
					} else {
						log.Printf("[mx-verifier] ✓ %s MX verified, domain activated", d.Domain)
					}
				} else {
					log.Printf("[mx-verifier] waiting: %s — %s", d.Domain, mxStatus)
				}
			}
		case <-recheckTicker.C:
			activeDomains, err := db.GetActiveDomains(context.Background())
			if err != nil {
				log.Printf("[mx-recheck] list active error: %v", err)
				continue
			}
			serverIP := domainH.GetServerIP()
			log.Printf("[mx-recheck] checking %d active domains", len(activeDomains))
			for _, d := range activeDomains {
				matched, _, mxStatus := store.CheckDomainMX(d.Domain, serverIP)
				db.TouchDomainCheckTime(context.Background(), d.ID)
				if !matched {
					if err := db.DisableDomainMX(context.Background(), d.ID); err != nil {
						log.Printf("[mx-recheck] disable %s error: %v", d.Domain, err)
					} else {
						log.Printf("[mx-recheck] ⚠ %s MX no longer valid (%s), domain disabled", d.Domain, mxStatus)
					}
				}
			}
		}
	}
}

func writeAdminKeyFile(ctx context.Context, db *store.Store, keyFile string) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(1 * time.Second):
	}
	adminKey, err := db.GetAdminAPIKey(context.Background())
	if err != nil {
		log.Printf("[adminkey] could not fetch admin key: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0700); err != nil {
		return
	}
	content := "# TempMail Admin API Key\n# Auto-generated on startup — keep this secret!\n\nADMIN_API_KEY=" + adminKey + "\n"
	if err := os.WriteFile(keyFile, []byte(content), 0600); err != nil {
		log.Printf("[adminkey] write file error: %v", err)
		return
	}
	log.Printf("✓ Admin API Key written to %s", keyFile)
}
