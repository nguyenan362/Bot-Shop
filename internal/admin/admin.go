package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nguyenan362/bot-shop-go/internal/binance"
	"github.com/nguyenan362/bot-shop-go/internal/config"
	"github.com/nguyenan362/bot-shop-go/internal/models"
	"github.com/nguyenan362/bot-shop-go/internal/repository"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// AdminHandler handles admin web panel routes.
type AdminHandler struct {
	cfg         *config.Config
	pool        *pgxpool.Pool
	productRepo *repository.ProductRepo
	orderRepo   *repository.OrderRepo
	depositRepo *repository.DepositRepo
	noteRepo    *repository.NoteRepo
	userRepo    *repository.UserRepo
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(
	cfg *config.Config,
	pool *pgxpool.Pool,
	productRepo *repository.ProductRepo,
	orderRepo *repository.OrderRepo,
	depositRepo *repository.DepositRepo,
	noteRepo *repository.NoteRepo,
	userRepo *repository.UserRepo,
) *AdminHandler {
	return &AdminHandler{
		cfg:         cfg,
		pool:        pool,
		productRepo: productRepo,
		orderRepo:   orderRepo,
		depositRepo: depositRepo,
		noteRepo:    noteRepo,
		userRepo:    userRepo,
	}
}

// RegisterRoutes sets up admin routes.
func (h *AdminHandler) RegisterRoutes(app *fiber.App) {
	admin := app.Group("/admin")

	// Auth
	admin.Get("/login", h.loginPage)
	admin.Post("/login", h.loginSubmit)

	// Protected routes
	admin.Use(h.authMiddleware)

	admin.Get("/", h.dashboard)
	admin.Get("/dashboard", h.dashboard)

	// Products CRUD
	admin.Get("/products", h.listProducts)
	admin.Get("/products/new", h.newProduct)
	admin.Post("/products/new", h.createProduct)
	admin.Get("/products/:id/edit", h.editProduct)
	admin.Post("/products/:id/edit", h.updateProduct)
	admin.Post("/products/:id/delete", h.deleteProduct)
	admin.Get("/products/:id/accounts", h.productAccounts)
	admin.Post("/products/:id/accounts/upload", h.uploadAccounts)
	admin.Post("/products/:id/accounts/:aid/delete", h.deleteAccount)
	admin.Post("/products/:id/accounts/clear", h.clearUnusedAccounts)
	admin.Post("/products/:id/toggle", h.toggleProduct)

	// Notes
	admin.Get("/notes", h.listNotes)
	admin.Get("/notes/new", h.newNote)
	admin.Post("/notes/new", h.createNote)
	admin.Get("/notes/:id/edit", h.editNote)
	admin.Post("/notes/:id/edit", h.updateNote)
	admin.Post("/notes/:id/delete", h.deleteNote)

	// Orders & Users
	admin.Get("/orders", h.listOrders)
	admin.Get("/users", h.listUsers)
	admin.Get("/deposits", h.listDeposits)

	// Binance Config
	admin.Get("/binance", h.binanceConfig)
	admin.Post("/binance", h.updateBinanceConfig)
}

// ---- Auth ----

func (h *AdminHandler) loginPage(c *fiber.Ctx) error {
	// Check if coming from bot with tele_id
	teleIDStr := c.Query("tele_id")
	if teleIDStr != "" {
		teleID, err := strconv.ParseInt(teleIDStr, 10, 64)
		if err == nil && h.cfg.IsAdmin(teleID) {
			token := h.generateJWT(teleID)
			c.Cookie(&fiber.Cookie{
				Name:     "admin_token",
				Value:    token,
				HTTPOnly: true,
				Expires:  time.Now().Add(time.Duration(h.cfg.AdminSessionHrs) * time.Hour),
			})
			return c.Redirect("/admin/dashboard")
		}
	}
	return c.Render("admin/login", fiber.Map{})
}

func (h *AdminHandler) loginSubmit(c *fiber.Ctx) error {
	teleIDStr := c.FormValue("tele_id")
	teleID, err := strconv.ParseInt(teleIDStr, 10, 64)
	if err != nil || !h.cfg.IsAdmin(teleID) {
		return c.Render("admin/login", fiber.Map{"Error": "Invalid Telegram ID"})
	}

	token := h.generateJWT(teleID)
	c.Cookie(&fiber.Cookie{
		Name:     "admin_token",
		Value:    token,
		HTTPOnly: true,
		Expires:  time.Now().Add(time.Duration(h.cfg.AdminSessionHrs) * time.Hour),
	})
	return c.Redirect("/admin/dashboard")
}

func (h *AdminHandler) authMiddleware(c *fiber.Ctx) error {
	tokenStr := c.Cookies("admin_token")
	if tokenStr == "" {
		return c.Redirect("/admin/login")
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(h.cfg.AdminJWTSecret), nil
	})
	if err != nil || !token.Valid {
		return c.Redirect("/admin/login")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return c.Redirect("/admin/login")
	}

	teleIDf, ok := claims["tele_id"].(float64)
	if !ok {
		return c.Redirect("/admin/login")
	}

	teleID := int64(teleIDf)
	if !h.cfg.IsAdmin(teleID) {
		return c.Redirect("/admin/login")
	}

	c.Locals("admin_tele_id", teleID)
	return c.Next()
}

func (h *AdminHandler) generateJWT(teleID int64) string {
	claims := jwt.MapClaims{
		"tele_id": teleID,
		"exp":     time.Now().Add(time.Duration(h.cfg.AdminSessionHrs) * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(h.cfg.AdminJWTSecret))
	return signed
}

// ---- Dashboard ----

func (h *AdminHandler) dashboard(c *fiber.Ctx) error {
	stats, err := repository.GetStats(c.Context(), h.pool)
	if err != nil {
		log.Error().Err(err).Msg("get stats failed")
		stats = &repository.Stats{}
	}

	return c.Render("admin/dashboard", fiber.Map{
		"Title": "Dashboard",
		"Stats": stats,
	}, "partials/base")
}

// ---- Products ----

func (h *AdminHandler) listProducts(c *fiber.Ctx) error {
	products, err := h.productRepo.ListAll(c.Context())
	if err != nil {
		return c.Status(500).SendString("Error loading products")
	}
	return c.Render("admin/products", fiber.Map{
		"Title":    "Products",
		"Products": products,
	}, "partials/base")
}

func (h *AdminHandler) newProduct(c *fiber.Ctx) error {
	return c.Render("admin/product_form", fiber.Map{
		"Title":  "New Product",
		"Action": "/admin/products/new",
	}, "partials/base")
}

func (h *AdminHandler) createProduct(c *fiber.Ctx) error {
	price, _ := decimal.NewFromString(c.FormValue("price_usdt"))

	p := &models.Product{
		NameVI:        c.FormValue("name_vi"),
		NameEN:        c.FormValue("name_en"),
		PriceUSDT:     price,
		Stock:         0,
		DescriptionVI: c.FormValue("description_vi"),
		DescriptionEN: c.FormValue("description_en"),
		Active:        c.FormValue("active") == "on",
	}

	if err := h.productRepo.Create(c.Context(), p); err != nil {
		return c.Render("admin/product_form", fiber.Map{
			"Title":   "New Product",
			"Action":  "/admin/products/new",
			"Error":   err.Error(),
			"Product": p,
		}, "partials/base")
	}
	return c.Redirect("/admin/products")
}

func (h *AdminHandler) editProduct(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	p, err := h.productRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(404).SendString("Product not found")
	}
	return c.Render("admin/product_form", fiber.Map{
		"Title":   "Edit Product",
		"Action":  "/admin/products/" + c.Params("id") + "/edit",
		"Product": p,
	}, "partials/base")
}

func (h *AdminHandler) updateProduct(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	price, _ := decimal.NewFromString(c.FormValue("price_usdt"))

	// Get current product to preserve stock
	current, err := h.productRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(404).SendString("Product not found")
	}

	p := &models.Product{
		ID:            id,
		NameVI:        c.FormValue("name_vi"),
		NameEN:        c.FormValue("name_en"),
		PriceUSDT:     price,
		Stock:         current.Stock,
		DescriptionVI: c.FormValue("description_vi"),
		DescriptionEN: c.FormValue("description_en"),
		Active:        c.FormValue("active") == "on",
	}

	if err := h.productRepo.Update(c.Context(), p); err != nil {
		return c.Render("admin/product_form", fiber.Map{
			"Title":   "Edit Product",
			"Action":  "/admin/products/" + c.Params("id") + "/edit",
			"Error":   err.Error(),
			"Product": p,
		}, "partials/base")
	}
	return c.Redirect("/admin/products")
}

func (h *AdminHandler) deleteProduct(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	if err := h.productRepo.Delete(c.Context(), id); err != nil {
		return c.Status(500).SendString("Error deleting product")
	}
	return c.Redirect("/admin/products")
}

func (h *AdminHandler) productAccounts(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	product, err := h.productRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(404).SendString("Product not found")
	}

	filter := c.Query("filter", "all") // all, available, used
	accounts, _ := h.productRepo.ListAccounts(c.Context(), id, filter)
	available, _ := h.productRepo.CountAvailable(c.Context(), id)
	used, _ := h.productRepo.CountUsed(c.Context(), id)

	data := fiber.Map{
		"Title":     "Accounts — " + product.NameVI,
		"Product":   product,
		"Accounts":  accounts,
		"Available": available,
		"Used":      used,
		"Total":     available + used,
		"Filter":    filter,
	}

	// Success message from upload redirect
	if added := c.Query("added"); added != "" {
		data["Success"] = "Đã thêm " + added + " tài khoản thành công!"
	}
	if deleted := c.Query("deleted"); deleted != "" {
		data["Success"] = "Đã xóa " + deleted + " tài khoản!"
	}

	return c.Render("admin/product_accounts", data, "partials/base")
}

func (h *AdminHandler) uploadAccounts(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))

	// Get file or text input
	accountsText := c.FormValue("accounts_text")
	if accountsText == "" {
		file, err := c.FormFile("accounts_file")
		if err == nil {
			f, err := file.Open()
			if err != nil {
				return c.Status(400).SendString("Cannot open file")
			}
			defer f.Close()
			buf := make([]byte, file.Size)
			f.Read(buf)
			accountsText = string(buf)
		}
	}

	if accountsText == "" {
		return c.Status(400).SendString("No accounts provided")
	}

	// Parse accounts (one per line)
	var accounts []string
	for _, line := range strings.Split(accountsText, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			accounts = append(accounts, line)
		}
	}

	count, err := h.productRepo.AddAccounts(c.Context(), id, accounts)
	if err != nil {
		return c.Status(500).SendString("Error adding accounts: " + err.Error())
	}

	return c.Redirect("/admin/products/" + c.Params("id") + "/accounts?added=" + strconv.Itoa(count))
}

func (h *AdminHandler) deleteAccount(c *fiber.Ctx) error {
	productID := c.Params("id")
	accountID, _ := strconv.Atoi(c.Params("aid"))

	if err := h.productRepo.DeleteAccount(c.Context(), accountID); err != nil {
		return c.Status(400).SendString("Error: " + err.Error())
	}

	// Decrement stock
	pid, _ := strconv.Atoi(productID)
	_ = h.productRepo.DeductStock(c.Context(), pid, 1)

	return c.Redirect("/admin/products/" + productID + "/accounts?deleted=1")
}

func (h *AdminHandler) clearUnusedAccounts(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	deleted, err := h.productRepo.DeleteAllUnusedAccounts(c.Context(), id)
	if err != nil {
		return c.Status(500).SendString("Error: " + err.Error())
	}
	return c.Redirect("/admin/products/" + c.Params("id") + "/accounts?deleted=" + strconv.Itoa(deleted))
}

func (h *AdminHandler) toggleProduct(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	product, err := h.productRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(404).SendString("Product not found")
	}
	product.Active = !product.Active
	if err := h.productRepo.Update(c.Context(), product); err != nil {
		return c.Status(500).SendString("Error toggling product")
	}
	return c.Redirect("/admin/products")
}

// ---- Notes ----

func (h *AdminHandler) listNotes(c *fiber.Ctx) error {
	notes, err := h.noteRepo.ListAll(c.Context())
	if err != nil {
		return c.Status(500).SendString("Error loading notes")
	}
	return c.Render("admin/notes", fiber.Map{
		"Title": "Notes",
		"Notes": notes,
	}, "partials/base")
}

func (h *AdminHandler) newNote(c *fiber.Ctx) error {
	return c.Render("admin/note_form", fiber.Map{
		"Title":  "New Note",
		"Action": "/admin/notes/new",
	}, "partials/base")
}

func (h *AdminHandler) createNote(c *fiber.Ctx) error {
	n := &models.Note{
		ContentVI: c.FormValue("content_vi"),
		ContentEN: c.FormValue("content_en"),
		Active:    c.FormValue("active") == "on",
	}
	if err := h.noteRepo.Create(c.Context(), n); err != nil {
		return c.Render("admin/note_form", fiber.Map{
			"Title":  "New Note",
			"Action": "/admin/notes/new",
			"Error":  err.Error(),
			"Note":   n,
		}, "partials/base")
	}
	return c.Redirect("/admin/notes")
}

func (h *AdminHandler) editNote(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	notes, err := h.noteRepo.ListAll(c.Context())
	if err != nil {
		return c.Status(500).SendString("Error")
	}
	var note *models.Note
	for _, n := range notes {
		if n.ID == id {
			note = &n
			break
		}
	}
	if note == nil {
		return c.Status(404).SendString("Note not found")
	}
	return c.Render("admin/note_form", fiber.Map{
		"Title":  "Edit Note",
		"Action": "/admin/notes/" + c.Params("id") + "/edit",
		"Note":   note,
	}, "partials/base")
}

func (h *AdminHandler) updateNote(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	n := &models.Note{
		ID:        id,
		ContentVI: c.FormValue("content_vi"),
		ContentEN: c.FormValue("content_en"),
		Active:    c.FormValue("active") == "on",
	}
	if err := h.noteRepo.Update(c.Context(), n); err != nil {
		return c.Status(500).SendString("Error updating note")
	}
	return c.Redirect("/admin/notes")
}

func (h *AdminHandler) deleteNote(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	if err := h.noteRepo.Delete(c.Context(), id); err != nil {
		return c.Status(500).SendString("Error deleting note")
	}
	return c.Redirect("/admin/notes")
}

// ---- Orders ----

func (h *AdminHandler) listOrders(c *fiber.Ctx) error {
	orders, err := h.orderRepo.ListAll(c.Context(), 100)
	if err != nil {
		return c.Status(500).SendString("Error loading orders")
	}
	return c.Render("admin/orders", fiber.Map{
		"Title":  "Orders",
		"Orders": orders,
	}, "partials/base")
}

// ---- Users ----

func (h *AdminHandler) listUsers(c *fiber.Ctx) error {
	users, err := h.userRepo.ListAll(c.Context())
	if err != nil {
		return c.Status(500).SendString("Error loading users")
	}
	return c.Render("admin/users", fiber.Map{
		"Title": "Users",
		"Users": users,
	}, "partials/base")
}

// ---- Deposits ----

func (h *AdminHandler) listDeposits(c *fiber.Ctx) error {
	deposits, err := h.depositRepo.ListAll(c.Context(), 100)
	if err != nil {
		return c.Status(500).SendString("Error loading deposits")
	}
	return c.Render("admin/deposits", fiber.Map{
		"Title":    "Deposits",
		"Deposits": deposits,
	}, "partials/base")
}

// ---- Binance Config ----

func (h *AdminHandler) binanceConfig(c *fiber.Ctx) error {
	bc, err := h.productRepo.GetBinanceConfig(c.Context())
	if err != nil {
		bc = &models.BinanceConfig{}
	}
	return c.Render("admin/binance", fiber.Map{
		"Title":  "Binance Deposit Config",
		"Config": bc,
	}, "partials/base")
}

func (h *AdminHandler) updateBinanceConfig(c *fiber.Ctx) error {
	bc := &models.BinanceConfig{
		APIKey:         strings.TrimSpace(c.FormValue("api_key")),
		SecretKey:      strings.TrimSpace(c.FormValue("secret_key")),
		DepositAddress: strings.TrimSpace(c.FormValue("deposit_address")),
		DepositNetwork: strings.TrimSpace(c.FormValue("deposit_network")),
	}

	// Validate API key by making a test call to Binance
	if bc.APIKey != "" && bc.SecretKey != "" {
		binanceClient := binance.NewClient()
		_, err := binanceClient.GetDepositHistory(c.Context(), bc, 0)
		if err != nil {
			errMsg := err.Error()
			log.Warn().Str("error", errMsg).Msg("binance API key validation failed")
			return c.Render("admin/binance", fiber.Map{
				"Title":  "Binance Deposit Config",
				"Config": bc,
				"Error":  "Binance API key validation failed: " + errMsg + ". Please check your API key and secret.",
			}, "partials/base")
		}
	}

	if err := h.productRepo.UpdateBinanceConfig(c.Context(), bc); err != nil {
		return c.Render("admin/binance", fiber.Map{
			"Title":  "Binance Deposit Config",
			"Config": bc,
			"Error":  err.Error(),
		}, "partials/base")
	}

	return c.Render("admin/binance", fiber.Map{
		"Title":   "Binance Deposit Config",
		"Config":  bc,
		"Success": "Configuration updated and API key validated successfully!",
	}, "partials/base")
}
