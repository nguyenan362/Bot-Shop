package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mymmrac/telego"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/nguyenan362/bot-shop-go/internal/admin"
	"github.com/nguyenan362/bot-shop-go/internal/binance"
	"github.com/nguyenan362/bot-shop-go/internal/config"
	"github.com/nguyenan362/bot-shop-go/internal/handler"
	"github.com/nguyenan362/bot-shop-go/internal/repository"
	"github.com/nguyenan362/bot-shop-go/internal/service"
	"github.com/nguyenan362/bot-shop-go/pkg/middleware"
)

func main() {
	// ---- Logger ----
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if os.Getenv("ENV") != "production" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}

	// ---- Config ----
	cfg := config.Load()
	log.Info().Str("env", cfg.Env).Str("port", cfg.Port).Msg("starting bot-shop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ---- PostgreSQL ----
	var pool *pgxpool.Pool
	var err error
	for i := 0; i < 10; i++ {
		pool, err = pgxpool.New(ctx, cfg.DSN())
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				break
			}
			pool.Close()
		}
		log.Warn().Int("attempt", i+1).Err(err).Msg("waiting for PostgreSQL...")
		time.Sleep(3 * time.Second)
	}
	if pool == nil {
		log.Fatal().Msg("failed to connect to PostgreSQL after 10 attempts")
	}
	defer pool.Close()
	log.Info().Msg("connected to PostgreSQL")

	// Run migrations
	runMigrations(ctx, pool)

	// ---- Redis ----
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Redis")
	}
	defer rdb.Close()
	log.Info().Msg("connected to Redis")

	// ---- Repositories ----
	userRepo := repository.NewUserRepo(pool)
	productRepo := repository.NewProductRepo(pool)
	orderRepo := repository.NewOrderRepo(pool)
	depositRepo := repository.NewDepositRepo(pool)
	noteRepo := repository.NewNoteRepo(pool)

	// ---- Binance Client ----
	binanceClient := binance.NewClient()

	// ---- Service ----
	svc := service.NewShopService(userRepo, productRepo, orderRepo, depositRepo, noteRepo, rdb, binanceClient)

	// ---- Telegram Bot ----
	bot, err := telego.NewBot(cfg.BotToken)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create telegram bot")
	}

	botInfo, err := bot.GetMe(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get bot info")
	}
	log.Info().Str("bot", botInfo.Username).Msg("telegram bot initialized")

	// ---- Bot Handler ----
	botHandler := handler.NewBotHandler(bot, svc, cfg)

	// ---- Fiber App ----
	engine := html.New("./web/views", ".html")
	app := fiber.New(fiber.Config{
		Views:       engine,
		BodyLimit:   10 * 1024 * 1024, // 10MB for file uploads
		ReadTimeout: 10 * time.Second,
	})

	// Middleware
	middleware.Setup(app)

	// ---- Webhook route (Telegram) ----
	app.Post("/webhook", func(c *fiber.Ctx) error {
		var update telego.Update
		if err := json.Unmarshal(c.Body(), &update); err != nil {
			log.Error().Err(err).Msg("invalid webhook payload")
			return c.SendStatus(fiber.StatusBadRequest)
		}

		// Process asynchronously to not block webhook response
		go func() {
			bgCtx := context.Background()
			botHandler.HandleUpdate(bgCtx, update)
		}()

		return c.SendStatus(fiber.StatusOK)
	})

	// ---- Binance Deposit Poller (background) ----
	go svc.PollBinanceDeposits(ctx, func(teleID int64, amount decimal.Decimal) {
		botHandler.NotifyDeposit(context.Background(), teleID, amount)
	})
	log.Info().Msg("binance deposit poller started")

	// ---- Admin Panel Routes ----
	adminHandler := admin.NewAdminHandler(cfg, pool, productRepo, orderRepo, depositRepo, noteRepo, userRepo)
	adminHandler.RegisterRoutes(app)

	// ---- Health Check ----
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "time": time.Now().Unix()})
	})

	// ---- Set Webhook ----
	if cfg.WebhookURL != "" {
		webhookURL := cfg.WebhookURL + "/webhook"
		err = bot.SetWebhook(ctx, &telego.SetWebhookParams{
			URL:            webhookURL,
			SecretToken:    cfg.WebhookSecret,
			AllowedUpdates: []string{"message", "callback_query"},
		})
		if err != nil {
			log.Fatal().Err(err).Msg("failed to set webhook")
		}
		log.Info().Str("url", webhookURL).Msg("webhook set")
	}

	// ---- Graceful Shutdown ----
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	log.Info().Str("port", cfg.Port).Msg("server started")

	<-quit
	log.Info().Msg("shutting down...")
	cancel()
	app.Shutdown()
	log.Info().Msg("goodbye")
}

// runMigrations executes SQL migration files.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) {
	migrationFiles := []string{
		"migrations/001_init.sql",
		"migrations/002_binance_deposit_update.sql",
	}

	for _, file := range migrationFiles {
		f, err := os.Open(file)
		if err != nil {
			log.Warn().Err(err).Str("file", file).Msg("migration file not found, skipping")
			continue
		}

		sql, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			log.Error().Err(err).Str("file", file).Msg("read migration file failed")
			continue
		}

		_, err = pool.Exec(ctx, string(sql))
		if err != nil {
			log.Warn().Err(err).Str("file", file).Msg("migration exec (may already exist)")
		} else {
			log.Info().Str("file", file).Msg("migration applied")
		}
	}
}
