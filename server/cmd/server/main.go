// Package main is the aibao-server entrypoint. It loads config, initializes
// logger / DB / Redis / metrics, builds the HTTP router, starts the server,
// and blocks until SIGTERM/SIGINT triggers a graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/aibao/server/internal/api"
	"github.com/aibao/server/internal/api/middleware"
	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/gateway/sms"
	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/gateway/tts"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/config"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/pkg/idhash"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/pkg/phonecrypt"
	"github.com/aibao/server/internal/pkg/safehash"
	"github.com/aibao/server/internal/repository"
	authsvc "github.com/aibao/server/internal/service/auth"
	"github.com/aibao/server/internal/service/bootstrap"
	childsvc "github.com/aibao/server/internal/service/child"
	"github.com/aibao/server/internal/service/audio"
	costsvc "github.com/aibao/server/internal/service/cost"
	memorysvc "github.com/aibao/server/internal/service/memory"
	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/safety"
	storysvc "github.com/aibao/server/internal/service/story"
	"github.com/aibao/server/internal/worker"
	"github.com/aibao/server/internal/worker/handlers"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	// Doubao API key passthrough: the Makefile / ops scripts use the more
	// specific name AIBAO_LLM_DOUBAO_API_KEY. Map it onto AIBAO_LLM_API_KEY
	// (which config viper binds to llm.api_key) when the latter is unset.
	if os.Getenv("AIBAO_LLM_API_KEY") == "" {
		if k := os.Getenv("AIBAO_LLM_DOUBAO_API_KEY"); k != "" {
			_ = os.Setenv("AIBAO_LLM_API_KEY", k)
		}
	}

	configPath := os.Getenv("AIBAO_CONFIG")
	if configPath == "" {
		configPath = "config/config.dev.yaml"
	}
	// Plan 5 env fallbacks: viper-bound names use AIBAO_TTS_* / AIBAO_STORAGE_*
	// but ops scripts use the more specific names below. Map them in before
	// config.Load reads env so validation passes.
	envFallbacks := map[string]string{
		"AIBAO_TTS_GROUP_ID":        "AIBAO_TTS_MINIMAX_GROUP_ID",
		"AIBAO_TTS_API_KEY":         "AIBAO_TTS_MINIMAX_API_KEY",
		"AIBAO_STORAGE_SECRET_ID":   "AIBAO_STORAGE_COS_SECRET_ID",
		"AIBAO_STORAGE_SECRET_KEY":  "AIBAO_STORAGE_COS_SECRET_KEY",
		"AIBAO_STORAGE_BUCKET":      "AIBAO_STORAGE_COS_BUCKET",
		"AIBAO_STORAGE_REGION":      "AIBAO_STORAGE_COS_REGION",
		"AIBAO_STORAGE_APP_ID":      "AIBAO_STORAGE_COS_APPID",
	}
	for primary, fallback := range envFallbacks {
		if os.Getenv(primary) == "" {
			if v := os.Getenv(fallback); v != "" {
				_ = os.Setenv(primary, v)
			}
		}
	}

	cfg, vp, err := config.LoadWithViper(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Idempotent post-load shim: only fill cfg fields if still empty.
	if cfg.TTS.GroupID == "" {
		cfg.TTS.GroupID = os.Getenv("AIBAO_TTS_MINIMAX_GROUP_ID")
	}
	if cfg.TTS.APIKey == "" {
		cfg.TTS.APIKey = os.Getenv("AIBAO_TTS_MINIMAX_API_KEY")
	}
	if cfg.Storage.SecretID == "" {
		cfg.Storage.SecretID = os.Getenv("AIBAO_STORAGE_COS_SECRET_ID")
	}
	if cfg.Storage.SecretKey == "" {
		cfg.Storage.SecretKey = os.Getenv("AIBAO_STORAGE_COS_SECRET_KEY")
	}
	if cfg.Storage.Bucket == "" {
		cfg.Storage.Bucket = os.Getenv("AIBAO_STORAGE_COS_BUCKET")
	}
	if cfg.Storage.Region == "" {
		cfg.Storage.Region = os.Getenv("AIBAO_STORAGE_COS_REGION")
	}
	if cfg.Storage.AppID == "" {
		cfg.Storage.AppID = os.Getenv("AIBAO_STORAGE_COS_APPID")
	}

	lg, closeLog, err := logger.NewFromConfig(cfg.Server.LogDir, cfg.Server.LogLevel)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = closeLog() }()
	logger.SetDefault(lg)
	lg.Info("server.starting", "port", cfg.Server.Port, "log_level", cfg.Server.LogLevel)

	db, err := repository.NewDB(cfg.Postgres)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer repository.Close(db)

	if err := repository.RunMigrations(db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	rdb, err := repository.NewRedis(cfg.Redis)
	if err != nil {
		return fmt.Errorf("init redis: %w", err)
	}
	defer func() { _ = rdb.Close() }()

	// Build domain dependencies (Plan 2).
	hasher := safehash.New(cfg.Crypto.SafehashSalt)
	pcipher, err := phonecrypt.New(cfg.Crypto.PhoneAESKey)
	if err != nil {
		return fmt.Errorf("init phone cipher: %w", err)
	}
	jwtMgr := jwtauth.New(
		cfg.Auth.JWTSecret,
		time.Duration(cfg.Auth.AccessTTLMinutes)*time.Minute,
		time.Duration(cfg.Auth.RefreshTTLMinutes)*time.Minute,
	)

	userRepo := repository.NewUserRepo(db)
	childRepo := repository.NewChildRepo(db)
	codeStore := authsvc.NewRedisCodeStore(rdb)

	mockSMS := sms.NewMock()
	var smsSender authsvc.SMS
	switch cfg.SMS.Provider {
	case "mock", "":
		smsSender = mockSMS
	default:
		return fmt.Errorf("unknown sms provider: %s", cfg.SMS.Provider)
	}

	authService := authsvc.New(authsvc.Deps{
		Users:        userRepo,
		CodeStore:    codeStore,
		SMS:          smsSender,
		JWT:          jwtMgr,
		PhoneCipher:  pcipher,
		Hasher:       hasher,
		FixedDevCode: mockSMS.FixedCode(),
		CodeTTL:      time.Duration(cfg.SMS.CodeTTLSeconds) * time.Second,
		Cooldown:     time.Duration(cfg.SMS.ResendCooldownSec) * time.Second,
	})
	childService := childsvc.New(childRepo)

	authHandler := api.NewAuthHandler(authService)
	meHandler := api.NewMeHandler(userRepo)
	childHandler := api.NewChildHandler(childService)

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	m := metrics.New(reg)
	bm := metrics.NewBusiness(reg)

	// Plan 11B Thin Slice — wire cost recording (Recorder + Flusher + IDHasher).
	// PriceBook is loaded from the same viper instance (cost.entries sub-tree).
	pb, err := pkgcost.LoadFromViper(vp)
	if err != nil {
		return fmt.Errorf("pricebook: %w", err)
	}
	idHasher := idhash.New(cfg.IDHash.Secret)
	costRecorder := costsvc.NewRecorder(pb, bm)
	costFlusher := costsvc.NewFlusher(costRecorder, db, bm)
	costCtx, costCancel := context.WithCancel(context.Background())
	defer costCancel()
	go costFlusher.Run(costCtx)
	lg.Info("cost.recorder.wired", "price_version", pb.Version())

	// TTS client (Plan 5)
	var ttsClient tts.Client
	switch cfg.TTS.Provider {
	case "minimax", "":
		ttsClient, err = tts.NewMinimax(tts.MinimaxConfig{
			BaseURL:        cfg.TTS.BaseURL,
			GroupID:        cfg.TTS.GroupID,
			APIKey:         cfg.TTS.APIKey,
			TimeoutSeconds: cfg.TTS.TimeoutSeconds,
		})
		if err != nil {
			return fmt.Errorf("init minimax tts: %w", err)
		}
	case "mock":
		ttsClient = tts.NewMock()
	default:
		return fmt.Errorf("unknown tts provider: %s", cfg.TTS.Provider)
	}

	// Storage client (Plan 5)
	var storageClient storage.Client
	switch cfg.Storage.Provider {
	case "cos", "":
		storageClient, err = storage.NewCOS(storage.COSConfig{
			Bucket:        cfg.Storage.Bucket,
			Region:        cfg.Storage.Region,
			AppID:         cfg.Storage.AppID,
			SecretID:      cfg.Storage.SecretID,
			SecretKey:     cfg.Storage.SecretKey,
			UploadTimeout: time.Duration(cfg.Storage.UploadTimeoutSec) * time.Second,
		})
		if err != nil {
			return fmt.Errorf("init cos: %w", err)
		}
	case "mock":
		storageClient = storage.NewMock()
	default:
		return fmt.Errorf("unknown storage provider: %s", cfg.Storage.Provider)
	}

	// LLM client (Plan 4): switch by provider; doubao requires API key.
	var llmClient llm.Client
	switch cfg.LLM.Provider {
	case "mock":
		llmClient = llm.NewMock()
	case "doubao", "":
		dc, err := llm.NewDoubao(llm.DoubaoConfig{
			APIKey:         cfg.LLM.APIKey,
			BaseURL:        cfg.LLM.BaseURL,
			TimeoutSeconds: cfg.LLM.TimeoutSeconds,
		})
		if err != nil {
			return fmt.Errorf("init doubao: %w", err)
		}
		llmClient = dc
	default:
		return fmt.Errorf("unknown llm provider: %s", cfg.LLM.Provider)
	}

	budget := llm.NewBudgetGate(rdb, llm.BudgetConfig{
		DailyLimitYuan:     cfg.LLM.DailyBudgetYuan,
		PriceInputPerMTok:  cfg.LLM.PriceInputPerMTok,
		PriceOutputPerMTok: cfg.LLM.PriceOutputPerMTok,
	})

	storyRepo := repository.NewStoryRepo(db)
	memoryRepo := repository.NewMemoryRepo(db)
	outboxRepo := repository.NewOutboxRepo(db)
	storylineRepo := repository.NewStorylineRepo(db)

	rs, err := safety.LoadRules("safety/rules.yaml", "safety/ip_whitelist.yaml", "safety/ip_blacklist.yaml")
	if err != nil {
		return fmt.Errorf("load safety rules: %w", err)
	}
	intentProvider := safety.NewLLMIntentProvider(llmClient, cfg.LLM.IntentModel, bm)
	preChecker := safety.NewPreChecker(rs, intentProvider)
	postChecker := safety.NewPostChecker(rs)

	// Plan 6: memory summarizer (post-story LLM) + selector (pre-story injection).
	memorySummarizer := memorysvc.NewSummarizer(llmClient, cfg.LLM.IntentModel, 0.3, bm, lg).WithCost(costRecorder, idHasher)
	memorySelector := memorysvc.NewSelector(memoryRepo, 3, lg)
	bootstrapSvc := bootstrap.NewService(childService, llmClient, cfg.LLM.IntentModel, 0.5, bm, lg)
	bootstrapHandler := api.NewBootstrapHandler(bootstrapSvc)

	// Plan 8: storyline context builder + chapter-hook extractor for sequels.
	chapterHook := storysvc.NewChapterHookExtractor(llmClient, cfg.LLM.IntentModel, 0.4, bm, lg).WithCost(costRecorder, idHasher)
	storylineCtxBld := storysvc.NewStorylineContextBuilder(storylineRepo, storyRepo, memoryRepo, lg)

	// Plan 11A — outline cache/events + resolver must be wired before the
	// story orchestrator so Step 0 HydrateFromOutline can resolve outline_id.
	outlineCache := outline.NewCache(rdb)
	outlineEvents := outline.NewEventStore(db)
	outlineResolver := outline.NewResolver(outlineCache, outlineEvents)

	orch, err := storysvc.NewOrchestrator(storysvc.Deps{
		Stories:         storyRepo,
		Children:        childRepo,
		LLM:             llmClient,
		Budget:          budget,
		PreCheck:        preChecker,
		PostCheck:       postChecker,
		MemorySelector:  memorySelector,
		Storylines:      storylineRepo,
		StorylineCtxBld: storylineCtxBld,
		ChapterHook:     chapterHook,
		Biz:             bm,
		Recorder:        costRecorder,
		IDHasher:        idHasher,
		PromptTmpl:      "safety/system_prompt.tmpl",
		FallbackDir:     "safety/fallback_stories",
		StoryModel:      cfg.LLM.StoryModel,
		Temperature:     cfg.LLM.StoryTemperature,
		PromptVersion:   "v1",
		// Plan 11A — outline preview integration (spec §7.3 / §7.5 N5).
		OutlineResolver: outlineResolver,
		OutlineEvents:   outlineEvents,
	})
	if err != nil {
		return fmt.Errorf("init orchestrator: %w", err)
	}
	storyHandler := api.NewStoryHandler(orch, storyRepo, childRepo)

	audioHandler := api.NewAudioHandler(
		storyRepo, childRepo, storageClient,
		time.Duration(cfg.Storage.PresignedTTLSec)*time.Second,
	)

	// Plan 8: heartbeat handler — time-aware greeting + active storylines list.
	heartbeatHandler := api.NewHeartbeatHandler(childRepo, storylineRepo, time.Now)

	counter := middleware.NewRedisCounter(rdb)
	genRateLimit := middleware.GenerateRateLimit(counter, cfg.LLM.GenerateRPM, time.Minute)
	budgetGuard := middleware.BudgetGuard(budget)

	// Plan 11A — outline preview + refresh share a per-user 5/min bucket
	// (spec §6.4). Wiring lives next to the other rate limits for clarity.
	outlineMatcher := safety.NewKeywordMatcher(rs.AllRedlinesFlat)
	outlineSvc := outline.NewService(outline.Deps{
		LLM:      llmClient,
		LLMModel: cfg.LLM.IntentModel,
		Matcher:  outlineMatcher,
		PreCheck: preChecker,
		Cache:    outlineCache,
		Events:   outlineEvents,
		Recorder: costRecorder,
		IDHasher: idHasher,
		Biz:      bm,
	})
	outlineHandler := api.NewOutlineHandler(outlineSvc, outlineCache, outlineEvents, childRepo)
	outlineRateLimit := middleware.PerUserRateLimit(counter, "outline", 5, time.Minute)

	router := api.NewRouter(api.RouterDeps{
		Metrics:      m,
		Reg:          reg,
		PG:           pgChecker{db: db},
		Redis:        redisChecker{c: rdb},
		JWT:          jwtMgr,
		Auth:         authHandler,
		Me:           meHandler,
		Child:        childHandler,
		Story:        storyHandler,
		GenRateLimit: genRateLimit,
		BudgetGuard:  budgetGuard,
		Audio:        audioHandler,
		Bootstrap:        bootstrapHandler,
		Heartbeat:        heartbeatHandler,
		Outline:          outlineHandler,
		OutlineRateLimit: outlineRateLimit,
	})

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	if cfg.Worker.Enabled {
		w := worker.New(outboxRepo, worker.Config{
			PollInterval: time.Duration(cfg.Worker.PollIntervalSec) * time.Second,
			BatchSize:    cfg.Worker.BatchSize,
			MaxAttempts:  cfg.Worker.MaxAttempts,
			BackoffBase:  time.Duration(cfg.Worker.BackoffBaseSeconds) * time.Second,
			BackoffMax:   time.Duration(cfg.Worker.BackoffMaxSeconds) * time.Second,
		})
		w.Register(model.EventTypeMemoryUpdate, handlers.NewMemoryUpdateHandler(memoryRepo, storyRepo, memorySummarizer))
		// Plan 7: audio orchestrator (TTS + BGM mixing) wiring.
		if _, lookErr := exec.LookPath(cfg.Audio.FFmpegPath); lookErr != nil {
			lg.Warn("ffmpeg.lookup.miss; stories will degrade to TTS-only",
				"configured_path", cfg.Audio.FFmpegPath, "err", lookErr.Error())
		}
		bgmRepo := repository.NewBGMRepo(db)
		bgmCache := audio.NewBGMCache(storageClient, cfg.Audio.BGMCacheDir)
		ffmpegMixer := audio.NewFFmpegMixer(
			cfg.Audio.FFmpegPath,
			cfg.Audio.BGMVolumeDB,
			cfg.Audio.OutputSampleRate,
			cfg.Audio.OutputBitrateKbps,
			time.Duration(cfg.Audio.MixTimeoutSeconds)*time.Second,
		)
		audioOrch := audio.NewOrchestrator(ttsClient, bgmRepo, bgmCache, ffmpegMixer, bm)

		ttsHandler := handlers.NewTTSSynthesisHandler(
			storyRepo, storyRepo,
			audioOrch, storageClient,
			handlers.TTSHandlerConfig{
				Provider:   cfg.TTS.Provider,
				Model:      cfg.TTS.Model,
				VoiceID:    cfg.TTS.VoiceID,
				Format:     cfg.TTS.Format,
				SampleRate: cfg.TTS.SampleRate,
				Bitrate:    cfg.TTS.Bitrate,
				Speed:      cfg.TTS.Speed,
			},
			bm,
		).WithCost(costRecorder, idHasher, "tencent_cos", "hk-standard")
		w.Register(model.EventTypeTTSSynthesis, ttsHandler)
		go w.Run(workerCtx)
		lg.Info("worker.enabled", "poll_sec", cfg.Worker.PollIntervalSec)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		lg.Info("server.listen", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
		lg.Info("server.shutdown.signal")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Error("server.shutdown.error", "err", err)
		return err
	}
	lg.Info("server.shutdown.done")
	return nil
}

// pgChecker adapts *gorm.DB to the api.Checker interface for /ready.
type pgChecker struct{ db *gorm.DB }

// Check pings the underlying Postgres connection.
func (p pgChecker) Check(ctx context.Context) error { return repository.Ping(ctx, p.db) }

// redisChecker adapts *redis.Client to the api.Checker interface for /ready.
type redisChecker struct{ c *redis.Client }

// Check pings the underlying Redis connection.
func (r redisChecker) Check(ctx context.Context) error { return repository.PingRedis(ctx, r.c) }
