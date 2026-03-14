package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nguyenan362/bot-shop-go/internal/auth"
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
	admin.Post("/users/:tele_id/toggle-admin", h.toggleUserAdmin)
	admin.Get("/deposits", h.listDeposits)

	// Binance Config
	admin.Get("/binance", h.binanceConfig)
	admin.Post("/binance", h.updateBinanceConfig)
}

// ---- Auth ----

func (h *AdminHandler) loginPage(c *fiber.Ctx) error {
	// Check if coming from bot with a signed JWT token
	tokenStr := c.Query("token")
	if tokenStr != "" {
		teleID, err := auth.ValidateLoginToken(tokenStr, h.cfg.AdminJWTSecret)
		if err != nil {
			log.Warn().Err(err).Msg("invalid admin login token")
			return c.Render("admin/login", fiber.Map{"Error": "Liên kết đăng nhập không hợp lệ hoặc đã hết hạn."})
		}

		if !h.hasAdminAccess(c.Context(), teleID) {
			return c.Render("admin/login", fiber.Map{"Error": "Bạn không có quyền truy cập quản trị."})
		}

		// Generate a session token (longer-lived) for the cookie
		sessionToken, err := auth.GenerateSessionToken(teleID, h.cfg.AdminJWTSecret, h.cfg.AdminSessionHrs)
		if err != nil {
			log.Error().Err(err).Msg("generate session token failed")
			return c.Render("admin/login", fiber.Map{"Error": "Lỗi hệ thống. Vui lòng thử lại."})
		}

		c.Cookie(&fiber.Cookie{
			Name:     "admin_token",
			Value:    sessionToken,
			HTTPOnly: true,
			Secure:   h.cfg.Env == "production",
			SameSite: "Lax",
			Expires:  time.Now().Add(time.Duration(h.cfg.AdminSessionHrs) * time.Hour),
		})
		return c.Redirect("/admin/dashboard")
	}

	return c.Render("admin/login", fiber.Map{})
}

func (h *AdminHandler) loginSubmit(c *fiber.Ctx) error {
	// Direct login via form is no longer allowed.
	// Admin must use the /admin command in the Telegram bot to receive a secure login link.
	return c.Render("admin/login", fiber.Map{"Error": "Đăng nhập trực tiếp đã bị tắt. Vui lòng dùng lệnh /admin trong bot Telegram để nhận link đăng nhập an toàn."})
}

func (h *AdminHandler) authMiddleware(c *fiber.Ctx) error {
	tokenStr := c.Cookies("admin_token")
	if tokenStr == "" {
		return c.Redirect("/admin/login")
	}

	teleID, err := auth.ValidateSessionToken(tokenStr, h.cfg.AdminJWTSecret)
	if err != nil {
		// Clear invalid/expired cookie
		c.Cookie(&fiber.Cookie{
			Name:     "admin_token",
			Value:    "",
			HTTPOnly: true,
			Expires:  time.Now().Add(-1 * time.Hour),
		})
		return c.Redirect("/admin/login")
	}

	if !h.hasAdminAccess(c.Context(), teleID) {
		return c.Redirect("/admin/login")
	}

	c.Locals("admin_tele_id", teleID)
	return c.Next()
}

func (h *AdminHandler) hasAdminAccess(ctx context.Context, teleID int64) bool {
	if h.cfg.IsAdmin(teleID) {
		return true
	}

	user, err := h.userRepo.GetByID(ctx, teleID)
	if err != nil {
		return false
	}

	return user.IsAdmin
}

// ---- Dashboard ----

func (h *AdminHandler) dashboard(c *fiber.Ctx) error {
	stats, err := repository.GetStats(c.Context(), h.pool)
	if err != nil {
		log.Error().Err(err).Msg("get stats failed")
		stats = &repository.Stats{}
	}

	return c.Render("admin/dashboard", fiber.Map{
		"Title": "Tổng quan",
		"Stats": stats,
	}, "partials/base")
}

// ---- Products ----

func (h *AdminHandler) listProducts(c *fiber.Ctx) error {
	products, err := h.productRepo.ListAll(c.Context())
	if err != nil {
		return c.Status(500).SendString("Lỗi tải danh sách sản phẩm")
	}
	return c.Render("admin/products", fiber.Map{
		"Title":    "Sản phẩm",
		"Products": products,
	}, "partials/base")
}

func (h *AdminHandler) newProduct(c *fiber.Ctx) error {
	return c.Render("admin/product_form", fiber.Map{
		"Title":  "Thêm sản phẩm",
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
			"Title":   "Thêm sản phẩm",
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
		return c.Status(404).SendString("Không tìm thấy sản phẩm")
	}
	return c.Render("admin/product_form", fiber.Map{
		"Title":   "Sửa sản phẩm",
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
		return c.Status(404).SendString("Không tìm thấy sản phẩm")
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
			"Title":   "Sửa sản phẩm",
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
		return c.Status(500).SendString("Lỗi khi xóa sản phẩm")
	}
	return c.Redirect("/admin/products")
}

func (h *AdminHandler) productAccounts(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	product, err := h.productRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(404).SendString("Không tìm thấy sản phẩm")
	}

	filter := c.Query("filter", "all") // all, available, used
	accounts, _ := h.productRepo.ListAccounts(c.Context(), id, filter)
	available, _ := h.productRepo.CountAvailable(c.Context(), id)
	used, _ := h.productRepo.CountUsed(c.Context(), id)

	data := fiber.Map{
		"Title":     "Kho tài khoản — " + product.NameVI,
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
				return c.Status(400).SendString("Không thể mở tệp")
			}
			defer f.Close()
			buf := make([]byte, file.Size)
			f.Read(buf)
			accountsText = string(buf)
		}
	}

	if accountsText == "" {
		return c.Status(400).SendString("Chưa có dữ liệu tài khoản")
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
		return c.Status(500).SendString("Lỗi khi thêm tài khoản: " + err.Error())
	}

	return c.Redirect("/admin/products/" + c.Params("id") + "/accounts?added=" + strconv.Itoa(count))
}

func (h *AdminHandler) deleteAccount(c *fiber.Ctx) error {
	productID := c.Params("id")
	accountID, _ := strconv.Atoi(c.Params("aid"))

	if err := h.productRepo.DeleteAccount(c.Context(), accountID); err != nil {
		return c.Status(400).SendString("Lỗi: " + err.Error())
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
		return c.Status(500).SendString("Lỗi: " + err.Error())
	}
	return c.Redirect("/admin/products/" + c.Params("id") + "/accounts?deleted=" + strconv.Itoa(deleted))
}

func (h *AdminHandler) toggleProduct(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	product, err := h.productRepo.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(404).SendString("Không tìm thấy sản phẩm")
	}
	product.Active = !product.Active
	if err := h.productRepo.Update(c.Context(), product); err != nil {
		return c.Status(500).SendString("Lỗi khi đổi trạng thái sản phẩm")
	}
	return c.Redirect("/admin/products")
}

// ---- Notes ----

func (h *AdminHandler) listNotes(c *fiber.Ctx) error {
	notes, err := h.noteRepo.ListAll(c.Context())
	if err != nil {
		return c.Status(500).SendString("Lỗi tải danh sách lưu ý")
	}
	return c.Render("admin/notes", fiber.Map{
		"Title": "Lưu ý",
		"Notes": notes,
	}, "partials/base")
}

func (h *AdminHandler) newNote(c *fiber.Ctx) error {
	return c.Render("admin/note_form", fiber.Map{
		"Title":  "Thêm lưu ý",
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
			"Title":  "Thêm lưu ý",
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
		return c.Status(500).SendString("Lỗi")
	}
	var note *models.Note
	for _, n := range notes {
		if n.ID == id {
			note = &n
			break
		}
	}
	if note == nil {
		return c.Status(404).SendString("Không tìm thấy lưu ý")
	}
	return c.Render("admin/note_form", fiber.Map{
		"Title":  "Sửa lưu ý",
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
		return c.Status(500).SendString("Lỗi khi cập nhật lưu ý")
	}
	return c.Redirect("/admin/notes")
}

func (h *AdminHandler) deleteNote(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	if err := h.noteRepo.Delete(c.Context(), id); err != nil {
		return c.Status(500).SendString("Lỗi khi xóa lưu ý")
	}
	return c.Redirect("/admin/notes")
}

// ---- Orders ----

func (h *AdminHandler) listOrders(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))

	var (
		orders []models.Order
		err    error
	)

	if q == "" {
		orders, err = h.orderRepo.ListAll(c.Context(), 100)
	} else {
		orders, err = h.orderRepo.Search(c.Context(), q, 100)
	}

	if err != nil {
		return c.Status(500).SendString("Lỗi tải danh sách đơn hàng")
	}
	return c.Render("admin/orders", fiber.Map{
		"Title":  "Đơn hàng",
		"Orders": orders,
		"Query":  q,
	}, "partials/base")
}

// ---- Users ----

func (h *AdminHandler) listUsers(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))

	var (
		users []models.User
		err   error
	)

	if q == "" {
		users, err = h.userRepo.ListAll(c.Context())
	} else {
		users, err = h.userRepo.Search(c.Context(), q)
	}

	if err != nil {
		return c.Status(500).SendString("Lỗi tải danh sách người dùng")
	}

	for i := range users {
		if h.cfg.IsAdmin(users[i].TeleID) {
			users[i].IsAdmin = true
		}
	}

	data := fiber.Map{
		"Title": "Người dùng",
		"Users": users,
		"Query": q,
	}

	if success := c.Query("success"); success != "" {
		successText := map[string]string{
			"granted": "Đã cấp quyền admin cho người dùng.",
			"removed": "Đã gỡ quyền admin của người dùng.",
		}
		if msg, ok := successText[success]; ok {
			data["Success"] = msg
		}
	}
	if errMsg := c.Query("error"); errMsg != "" {
		errorText := map[string]string{
			"invalid_tele_id":     "Telegram ID không hợp lệ.",
			"user_not_found":      "Không tìm thấy người dùng.",
			"cannot_remove_self":  "Bạn không thể tự gỡ quyền admin của chính mình.",
			"managed_by_env":      "Tài khoản này đang được cấu hình.",
			"update_admin_failed": "Không thể cập nhật quyền admin.",
		}
		if msg, ok := errorText[errMsg]; ok {
			data["Error"] = msg
		}
	}

	if currentAdminID, ok := c.Locals("admin_tele_id").(int64); ok {
		data["CurrentAdminTeleID"] = currentAdminID
	}

	return c.Render("admin/users", data, "partials/base")
}

func (h *AdminHandler) toggleUserAdmin(c *fiber.Ctx) error {
	teleID, err := strconv.ParseInt(c.Params("tele_id"), 10, 64)
	if err != nil {
		return c.Redirect("/admin/users?error=invalid_tele_id")
	}

	user, err := h.userRepo.GetByID(c.Context(), teleID)
	if err != nil {
		return c.Redirect("/admin/users?error=user_not_found")
	}

	if h.cfg.IsAdmin(teleID) {
		return c.Redirect("/admin/users?error=managed_by_env")
	}

	if currentAdminID, ok := c.Locals("admin_tele_id").(int64); ok {
		if currentAdminID == teleID && user.IsAdmin {
			return c.Redirect("/admin/users?error=cannot_remove_self")
		}
	}

	newIsAdmin := !user.IsAdmin
	if err := h.userRepo.SetAdmin(c.Context(), teleID, newIsAdmin); err != nil {
		return c.Redirect("/admin/users?error=update_admin_failed")
	}

	if newIsAdmin {
		return c.Redirect("/admin/users?success=granted")
	}
	return c.Redirect("/admin/users?success=removed")
}

// ---- Deposits ----

func (h *AdminHandler) listDeposits(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))

	var (
		deposits []models.Deposit
		err      error
	)

	if q == "" {
		deposits, err = h.depositRepo.ListAll(c.Context(), 100)
	} else {
		deposits, err = h.depositRepo.Search(c.Context(), q, 100)
	}

	if err != nil {
		return c.Status(500).SendString("Lỗi tải danh sách nạp tiền")
	}
	return c.Render("admin/deposits", fiber.Map{
		"Title":    "Nạp tiền",
		"Deposits": deposits,
		"Query":    q,
	}, "partials/base")
}

// ---- Binance Config ----

func (h *AdminHandler) binanceConfig(c *fiber.Ctx) error {
	bc, err := h.productRepo.GetBinanceConfig(c.Context())
	if err != nil {
		bc = &models.BinanceConfig{}
	}
	return c.Render("admin/binance", fiber.Map{
		"Title":  "Cấu hình nạp Binance",
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
				"Title":  "Cấu hình nạp Binance",
				"Config": bc,
				"Error":  "Xác thực API Binance thất bại: " + errMsg + ". Vui lòng kiểm tra API key và secret.",
			}, "partials/base")
		}
	}

	if err := h.productRepo.UpdateBinanceConfig(c.Context(), bc); err != nil {
		return c.Render("admin/binance", fiber.Map{
			"Title":  "Cấu hình nạp Binance",
			"Config": bc,
			"Error":  err.Error(),
		}, "partials/base")
	}

	return c.Render("admin/binance", fiber.Map{
		"Title":   "Cấu hình nạp Binance",
		"Config":  bc,
		"Success": "Đã lưu cấu hình và xác thực API key thành công!",
	}, "partials/base")
}
