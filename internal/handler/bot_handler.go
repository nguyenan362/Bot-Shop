package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nguyenan362/bot-shop-go/internal/auth"
	"github.com/nguyenan362/bot-shop-go/internal/config"
	"github.com/nguyenan362/bot-shop-go/internal/i18n"
	"github.com/nguyenan362/bot-shop-go/internal/models"
	"github.com/nguyenan362/bot-shop-go/internal/service"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// BotHandler handles all Telegram bot interactions.
type BotHandler struct {
	bot *telego.Bot
	svc *service.ShopService
	cfg *config.Config
}

// NewBotHandler creates a new bot handler.
func NewBotHandler(bot *telego.Bot, svc *service.ShopService, cfg *config.Config) *BotHandler {
	return &BotHandler{bot: bot, svc: svc, cfg: cfg}
}

// HandleUpdate processes an incoming Telegram update.
func (h *BotHandler) HandleUpdate(ctx context.Context, update telego.Update) {
	if update.Message != nil {
		h.handleMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil {
		if h.isUserBanned(ctx, update.CallbackQuery.From.ID) {
			return
		}
		h.handleCallback(ctx, update.CallbackQuery)
	}
}

// handleMessage processes text messages.
func (h *BotHandler) handleMessage(ctx context.Context, msg *telego.Message) {
	teleID := msg.From.ID
	username := msg.From.Username

	// Ensure user exists
	isAdmin := h.cfg.IsAdmin(teleID)
	if err := h.svc.UserRepo.Upsert(ctx, teleID, username, isAdmin); err != nil {
		log.Error().Err(err).Int64("user", teleID).Msg("upsert user failed")
	}

	if h.isUserBanned(ctx, teleID) {
		return
	}

	// Auto-detect and save timezone from Telegram language_code
	if msg.From.LanguageCode != "" {
		tz := detectTimezone(msg.From.LanguageCode)
		_ = h.svc.UserRepo.UpdateTimezone(ctx, teleID, tz)
	}

	// Get user language
	lang := h.getUserLang(ctx, teleID)

	text := strings.TrimSpace(msg.Text)

	// Check for /start command
	if text == "/start" {
		h.sendStartGreeting(ctx, msg.Chat.ID, msg.From.FirstName, lang)
		return
	}

	// Check for /admin command
	if text == "/admin" {
		if h.hasTelegramAdminAccess(ctx, teleID) {
			h.sendAdminLink(ctx, msg.Chat.ID, teleID)
			return
		}
	}

	// Check for /tb broadcast command (admin only)
	if strings.HasPrefix(text, "/tb") {
		if !h.hasTelegramAdminAccess(ctx, teleID) {
			params := tu.Message(tu.ID(msg.Chat.ID), "❌ Bạn không có quyền sử dụng lệnh này.")
			h.bot.SendMessage(ctx, params)
			return
		}

		broadcastText := strings.TrimSpace(strings.TrimPrefix(text, "/tb"))
		if broadcastText == "" {
			params := tu.Message(tu.ID(msg.Chat.ID), "Cách dùng: /tb <nội dung thông báo>")
			h.bot.SendMessage(ctx, params)
			return
		}

		h.broadcastByAdminCommand(ctx, msg.Chat.ID, teleID, broadcastText)
		return
	}

	// Check button text matches
	switch text {
	case i18n.TSimple("vi", "btn_buy"), i18n.TSimple("en", "btn_buy"):
		h.handleBuyMenu(ctx, msg.Chat.ID, lang)
		return
	case i18n.TSimple("vi", "btn_profile"), i18n.TSimple("en", "btn_profile"):
		h.handleProfile(ctx, msg.Chat.ID, teleID, lang)
		return
	case i18n.TSimple("vi", "btn_deposit"), i18n.TSimple("en", "btn_deposit"):
		h.handleDepositPrompt(ctx, msg.Chat.ID, teleID, lang)
		return
	case i18n.TSimple("vi", "btn_support"), i18n.TSimple("en", "btn_support"):
		h.handleSupport(ctx, msg.Chat.ID, lang)
		return
	case i18n.TSimple("vi", "btn_notes"), i18n.TSimple("en", "btn_notes"):
		h.handleNotes(ctx, msg.Chat.ID, lang)
		return
	}

	// Check user state (awaiting input)
	state, data, _ := h.svc.GetUserState(ctx, teleID)
	switch state {
	case "await_quantity":
		h.handleQuantityInput(ctx, msg, teleID, lang, data)
		return
	case "await_txid":
		h.handleTxIDInput(ctx, msg, teleID, lang)
		return
	}

	// Default: show main menu
	h.sendMainMenu(ctx, msg.Chat.ID, lang)
}

// sendStartGreeting sends a greeting with language selection inline buttons.
func (h *BotHandler) sendStartGreeting(ctx context.Context, chatID int64, firstName string, lang string) {
	text := i18n.T(lang, "start_greeting", map[string]interface{}{
		"Name": firstName,
	})

	markup := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("🇻🇳 Tiếng Việt").WithCallbackData("lang:vi"),
			tu.InlineKeyboardButton("🇬🇧 English").WithCallbackData("lang:en"),
		),
	)

	params := tu.Message(tu.ID(chatID), text).
		WithReplyMarkup(markup).
		WithParseMode(telego.ModeMarkdown)

	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send start greeting failed")
	}
}

// sendMainMenu sends the 5-button main menu.
func (h *BotHandler) sendMainMenu(ctx context.Context, chatID int64, lang string) {
	keyboard := tu.Keyboard(
		tu.KeyboardRow(
			tu.KeyboardButton(i18n.TSimple(lang, "btn_buy")),
			tu.KeyboardButton(i18n.TSimple(lang, "btn_profile")),
		),
		tu.KeyboardRow(
			tu.KeyboardButton(i18n.TSimple(lang, "btn_deposit")), tu.KeyboardButton(i18n.TSimple(lang, "btn_support")),
		),
		tu.KeyboardRow(
			tu.KeyboardButton(i18n.TSimple(lang, "btn_notes")),
		),
	).WithResizeKeyboard()

	params := tu.Message(tu.ID(chatID), i18n.TSimple(lang, "welcome")).
		WithReplyMarkup(keyboard).
		WithParseMode(telego.ModeMarkdown)

	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send main menu failed")
	}
}

// handleProfile shows user profile.
func (h *BotHandler) handleProfile(ctx context.Context, chatID int64, teleID int64, lang string) {
	user, err := h.svc.UserRepo.GetByID(ctx, teleID)
	if err != nil {
		h.sendError(ctx, chatID, lang)
		return
	}

	text := i18n.T(lang, "profile_info", map[string]interface{}{
		"TeleID":   strconv.FormatInt(user.TeleID, 10),
		"Username": user.Username,
		"Balance":  user.BalanceUSDT.StringFixed(2),
		"JoinDate": user.JoinDate.Format("2006-01-02"),
	})

	params := tu.Message(tu.ID(chatID), text).WithParseMode(telego.ModeMarkdown)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send profile failed")
	}
}

// handleBuyMenu shows available products as inline keyboard.
func (h *BotHandler) handleBuyMenu(ctx context.Context, chatID int64, lang string) {
	products, err := h.svc.ProductRepo.ListActive(ctx)
	if err != nil {
		h.sendError(ctx, chatID, lang)
		return
	}

	if len(products) == 0 {
		params := tu.Message(tu.ID(chatID), i18n.TSimple(lang, "out_of_stock"))
		h.bot.SendMessage(ctx, params)
		return
	}

	var rows [][]telego.InlineKeyboardButton
	for _, p := range products {
		label := i18n.T(lang, "product_item", map[string]interface{}{
			"Name":  p.Name(lang),
			"Price": p.PriceUSDT.StringFixed(2),
			"Stock": strconv.Itoa(p.Stock),
		})
		rows = append(rows, []telego.InlineKeyboardButton{
			tu.InlineKeyboardButton(label).WithCallbackData(fmt.Sprintf("buy:%d", p.ID)),
		})
	}

	// Add back button
	rows = append(rows, []telego.InlineKeyboardButton{
		tu.InlineKeyboardButton(i18n.TSimple(lang, "btn_back")).WithCallbackData("back:main"),
	})

	markup := &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	params := tu.Message(tu.ID(chatID), i18n.TSimple(lang, "select_product")).
		WithReplyMarkup(markup).
		WithParseMode(telego.ModeMarkdown)

	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send buy menu failed")
	}
}

// handleDepositPrompt shows deposit instructions with wallet address and sets await_txid state.
func (h *BotHandler) handleDepositPrompt(ctx context.Context, chatID int64, teleID int64, lang string) {
	cfg, err := h.svc.GetDepositInfo(ctx)
	if err != nil {
		if err.Error() == "binance_not_configured" {
			params := tu.Message(tu.ID(chatID), i18n.TSimple(lang, "binance_not_configured"))
			h.bot.SendMessage(ctx, params)
			return
		}
		log.Error().Err(err).Msg("get deposit info failed")
		h.sendError(ctx, chatID, lang)
		return
	}

	text := i18n.T(lang, "deposit_instructions", map[string]interface{}{
		"Address": cfg.DepositAddress,
		"Network": cfg.DepositNetwork,
	})

	params := tu.Message(tu.ID(chatID), text).WithParseMode(telego.ModeMarkdown)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send deposit instructions failed")
	}

	// Set state so the bot waits for the user to send TxID
	_ = h.svc.SetUserState(ctx, teleID, "await_txid", "")
}

// handleSupport sends support information.
func (h *BotHandler) handleSupport(ctx context.Context, chatID int64, lang string) {
	adminUsername := "admin"
	if len(h.cfg.AdminTeleIDs) > 0 {
		adminUsername = "startshop666" // Would need to resolve username from tele_id
	}

	text := i18n.T(lang, "support_message", map[string]interface{}{
		"AdminUsername": adminUsername,
		"SupportLink":   "https://t.me/" + adminUsername,
	})

	params := tu.Message(tu.ID(chatID), text).WithParseMode(telego.ModeMarkdown)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send support failed")
	}
}

// handleNotes shows active notes.
func (h *BotHandler) handleNotes(ctx context.Context, chatID int64, lang string) {
	notes, err := h.svc.NoteRepo.ListActive(ctx)
	if err != nil {
		h.sendError(ctx, chatID, lang)
		return
	}

	if len(notes) == 0 {
		params := tu.Message(tu.ID(chatID), i18n.TSimple(lang, "no_notes"))
		h.bot.SendMessage(ctx, params)
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.TSimple(lang, "notes_header"))
	for idx, note := range notes {
		sb.WriteString(fmt.Sprintf("\n%d. %s", idx+1, note.Content(lang)))
	}

	params := tu.Message(tu.ID(chatID), sb.String()).WithParseMode(telego.ModeMarkdown)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send notes failed")
	}
}

// handleLanguageMenu shows language selection.
func (h *BotHandler) handleLanguageMenu(ctx context.Context, chatID int64, lang string) {
	markup := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("🇻🇳 Tiếng Việt").WithCallbackData("lang:vi"),
			tu.InlineKeyboardButton("🇬🇧 English").WithCallbackData("lang:en"),
		),
	)

	params := tu.Message(tu.ID(chatID), i18n.TSimple(lang, "language_select")).
		WithReplyMarkup(markup)

	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send language menu failed")
	}
}

// handleCallback processes inline keyboard callback queries.
func (h *BotHandler) handleCallback(ctx context.Context, cb *telego.CallbackQuery) {
	teleID := cb.From.ID
	lang := h.getUserLang(ctx, teleID)
	data := cb.Data

	// Always answer callback to prevent loading state
	h.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
	})

	switch {
	case strings.HasPrefix(data, "buy:"):
		h.handleBuySelect(ctx, cb, teleID, lang, data)
	case strings.HasPrefix(data, "confirm_buy:"):
		h.handleBuyConfirm(ctx, cb, teleID, lang, data)
	case strings.HasPrefix(data, "lang:"):
		h.handleLanguageChange(ctx, cb, teleID, data)
	case data == "cancel":
		h.handleCancel(ctx, cb, teleID, lang)
	case data == "back:main":
		h.sendMainMenu(ctx, cb.Message.GetChat().ID, lang)
	}
}

// handleBuySelect handles product selection → asks for quantity.
func (h *BotHandler) handleBuySelect(ctx context.Context, cb *telego.CallbackQuery, teleID int64, lang string, data string) {
	productIDStr := strings.TrimPrefix(data, "buy:")
	productID, err := strconv.Atoi(productIDStr)
	if err != nil {
		return
	}

	product, err := h.svc.ProductRepo.GetByID(ctx, productID)
	if err != nil {
		h.sendError(ctx, cb.Message.GetChat().ID, lang)
		return
	}

	// Set user state: awaiting quantity for this product
	_ = h.svc.SetUserState(ctx, teleID, "await_quantity", productIDStr)

	text := i18n.T(lang, "enter_quantity", map[string]interface{}{
		"ProductName": product.Name(lang),
		"Price":       product.PriceUSDT.StringFixed(2),
		"Stock":       strconv.Itoa(product.Stock),
	})

	params := tu.Message(tu.ID(cb.Message.GetChat().ID), text).WithParseMode(telego.ModeMarkdown)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send quantity prompt failed")
	}
}

// handleQuantityInput processes the quantity entered by user.
func (h *BotHandler) handleQuantityInput(ctx context.Context, msg *telego.Message, teleID int64, lang string, productIDStr string) {
	defer h.svc.ClearUserState(ctx, teleID)

	qty, err := strconv.Atoi(strings.TrimSpace(msg.Text))
	if err != nil || qty <= 0 {
		params := tu.Message(tu.ID(msg.Chat.ID), i18n.TSimple(lang, "invalid_quantity"))
		h.bot.SendMessage(ctx, params)
		return
	}

	productID, _ := strconv.Atoi(productIDStr)
	product, err := h.svc.ProductRepo.GetByID(ctx, productID)
	if err != nil {
		h.sendError(ctx, msg.Chat.ID, lang)
		return
	}

	user, err := h.svc.UserRepo.GetByID(ctx, teleID)
	if err != nil {
		h.sendError(ctx, msg.Chat.ID, lang)
		return
	}

	total := product.PriceUSDT.Mul(decimal.NewFromInt(int64(qty)))

	// Show confirmation
	text := i18n.T(lang, "confirm_buy", map[string]interface{}{
		"ProductName": product.Name(lang),
		"Quantity":    strconv.Itoa(qty),
		"Total":       total.StringFixed(2),
		"Balance":     user.BalanceUSDT.StringFixed(2),
	})

	markup := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(i18n.TSimple(lang, "btn_confirm")).
				WithCallbackData(fmt.Sprintf("confirm_buy:%d:%d", productID, qty)),
			tu.InlineKeyboardButton(i18n.TSimple(lang, "btn_cancel")).
				WithCallbackData("cancel"),
		),
	)

	params := tu.Message(tu.ID(msg.Chat.ID), text).
		WithReplyMarkup(markup).
		WithParseMode(telego.ModeMarkdown)

	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send confirmation failed")
	}
}

// handleBuyConfirm processes the buy confirmation.
func (h *BotHandler) handleBuyConfirm(ctx context.Context, cb *telego.CallbackQuery, teleID int64, lang string, data string) {
	parts := strings.Split(strings.TrimPrefix(data, "confirm_buy:"), ":")
	if len(parts) != 2 {
		return
	}

	productID, _ := strconv.Atoi(parts[0])
	qty, _ := strconv.Atoi(parts[1])

	chatID := cb.Message.GetChat().ID

	result, err := h.svc.BuyAccounts(ctx, teleID, productID, qty)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.HasPrefix(errMsg, "out_of_stock:"):
			available := strings.TrimPrefix(errMsg, "out_of_stock:")
			text := i18n.T(lang, "out_of_stock", map[string]interface{}{
				"Available": available,
			})
			h.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), text))
		case strings.Contains(errMsg, "insufficient balance") || strings.HasPrefix(errMsg, "deduct_balance:"):
			user, _ := h.svc.UserRepo.GetByID(ctx, teleID)
			product, _ := h.svc.ProductRepo.GetByID(ctx, productID)
			total := product.PriceUSDT.Mul(decimal.NewFromInt(int64(qty)))
			text := i18n.T(lang, "insufficient_balance", map[string]interface{}{
				"Required": total.StringFixed(2),
				"Balance":  user.BalanceUSDT.StringFixed(2),
			})
			h.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), text))
		default:
			log.Error().Err(err).Msg("buy failed")
			h.sendError(ctx, chatID, lang)
		}
		return
	}

	// Send success message
	product, _ := h.svc.ProductRepo.GetByID(ctx, productID)
	text := i18n.T(lang, "order_success", map[string]interface{}{
		"OrderID":     strconv.FormatInt(result.Order.ID, 10),
		"ProductName": product.Name(lang),
		"Quantity":    strconv.Itoa(qty),
		"Total":       result.Order.TotalUSDT.StringFixed(2),
	})

	params := tu.Message(tu.ID(chatID), text).WithParseMode(telego.ModeMarkdown)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send order success failed")
	}

	// Send account file
	file := tu.Document(tu.ID(chatID),
		telego.InputFile{
			File: tu.NameBytes(result.FileData, result.FileName),
		},
	)
	if _, err := h.bot.SendDocument(ctx, file); err != nil {
		log.Error().Err(err).Msg("send document failed")
	}

	if product != nil {
		h.autoNotifyOutOfStock(ctx, productID, product.NameEN, teleID)
	}
}

// handleLanguageChange changes user language.
func (h *BotHandler) handleLanguageChange(ctx context.Context, cb *telego.CallbackQuery, teleID int64, data string) {
	lang := strings.TrimPrefix(data, "lang:")
	if lang != "vi" && lang != "en" {
		return
	}

	if err := h.svc.UserRepo.UpdateLanguage(ctx, teleID, lang); err != nil {
		log.Error().Err(err).Msg("update language failed")
		return
	}

	text := i18n.TSimple(lang, "language_changed")
	params := tu.Message(tu.ID(cb.Message.GetChat().ID), text)
	h.bot.SendMessage(ctx, params)

	// Resend main menu with new language
	h.sendMainMenu(ctx, cb.Message.GetChat().ID, lang)
}

// handleCancel cancels current operation.
func (h *BotHandler) handleCancel(ctx context.Context, cb *telego.CallbackQuery, teleID int64, lang string) {
	_ = h.svc.ClearUserState(ctx, teleID)

	params := tu.Message(tu.ID(cb.Message.GetChat().ID), i18n.TSimple(lang, "order_cancelled"))
	h.bot.SendMessage(ctx, params)
}

// sendAdminLink sends admin panel link with a secure, short-lived JWT token.
func (h *BotHandler) sendAdminLink(ctx context.Context, chatID int64, teleID int64) {
	// Generate a short-lived login token (1 minute)
	token, err := auth.GenerateLoginToken(teleID, h.cfg.AdminJWTSecret, 1*time.Minute)
	if err != nil {
		log.Error().Err(err).Msg("generate admin login token failed")
		h.sendError(ctx, chatID, "en")
		return
	}

	link := fmt.Sprintf("%s/admin/login?token=%s", h.cfg.WebhookURL, token)
	text := fmt.Sprintf("🔧 Admin Panel:\n%s", link)

	params := tu.Message(tu.ID(chatID), text)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send admin link failed")
	}
}

func (h *BotHandler) hasTelegramAdminAccess(ctx context.Context, teleID int64) bool {
	if h.cfg.IsAdmin(teleID) {
		return true
	}

	user, err := h.svc.UserRepo.GetByID(ctx, teleID)
	if err != nil {
		return false
	}

	return user.IsAdmin
}

func (h *BotHandler) broadcastByAdminCommand(ctx context.Context, chatID int64, adminTeleID int64, text string) {
	users, err := h.svc.UserRepo.ListAllUserLangs(ctx)
	if err != nil {
		log.Error().Err(err).Int64("admin", adminTeleID).Msg("tb command: failed to list users")
		params := tu.Message(tu.ID(chatID), "❌ Không thể lấy danh sách người dùng.")
		h.bot.SendMessage(ctx, params)
		return
	}

	if len(users) == 0 {
		params := tu.Message(tu.ID(chatID), "ℹ️ Chưa có người dùng nào để gửi thông báo.")
		h.bot.SendMessage(ctx, params)
		return
	}

	sent := 0
	failed := 0

	for _, u := range users {
		params := tu.Message(tu.ID(u.TeleID), text)
		if _, err := h.bot.SendMessage(ctx, params); err != nil {
			failed++
			log.Warn().Err(err).Int64("user", u.TeleID).Int64("admin", adminTeleID).Msg("tb command: send message failed")
		} else {
			sent++
		}

		time.Sleep(50 * time.Millisecond)
	}

	result := fmt.Sprintf("✅ Đã gửi thông báo. Thành công: %d | Thất bại: %d", sent, failed)
	params := tu.Message(tu.ID(chatID), result)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Int64("admin", adminTeleID).Msg("tb command: send result failed")
	}
}

func (h *BotHandler) autoNotifyOutOfStock(ctx context.Context, productID int, productName string, buyerTeleID int64) {
	available, err := h.svc.ProductRepo.CountAvailable(ctx, productID)
	if err != nil {
		log.Error().Err(err).Int("product_id", productID).Msg("auto out-of-stock: count available failed")
		return
	}

	if available > 0 {
		_ = h.svc.Redis.Del(ctx, fmt.Sprintf("product:out_of_stock_notified:%d", productID)).Err()
		return
	}

	lockKey := fmt.Sprintf("product:out_of_stock_notified:%d", productID)
	locked, err := h.svc.Redis.SetNX(ctx, lockKey, "1", 0).Result()
	if err != nil {
		log.Error().Err(err).Int("product_id", productID).Msg("auto out-of-stock: setnx failed")
		return
	}
	if !locked {
		return
	}

	if strings.TrimSpace(productName) == "" {
		if p, getErr := h.svc.ProductRepo.GetByID(ctx, productID); getErr == nil && p != nil {
			productName = p.NameEN
			if strings.TrimSpace(productName) == "" {
				productName = p.NameVI
			}
		}
	}
	if strings.TrimSpace(productName) == "" {
		productName = fmt.Sprintf("Product #%d", productID)
	}

	users, err := h.svc.UserRepo.ListAllUserLangs(ctx)
	if err != nil {
		log.Error().Err(err).Int("product_id", productID).Msg("auto out-of-stock: list users failed")
		return
	}

	message := fmt.Sprintf("Oh no! %s out of stock", productName)
	sent := 0
	failed := 0

	for _, u := range users {
		params := tu.Message(tu.ID(u.TeleID), message)
		if _, err := h.bot.SendMessage(ctx, params); err != nil {
			failed++
			log.Warn().Err(err).Int64("user", u.TeleID).Int("product_id", productID).Msg("auto out-of-stock: send message failed")
		} else {
			sent++
		}
		time.Sleep(50 * time.Millisecond)
	}

	log.Info().Int("product_id", productID).Int64("buyer", buyerTeleID).Int("sent", sent).Int("failed", failed).Msg("auto out-of-stock notification sent")
}

// sendError sends a generic error message.
func (h *BotHandler) sendError(ctx context.Context, chatID int64, lang string) {
	params := tu.Message(tu.ID(chatID), i18n.TSimple(lang, "error_general"))
	h.bot.SendMessage(ctx, params)
}

// getUserLang gets user language, defaults to "vi".
func (h *BotHandler) getUserLang(ctx context.Context, teleID int64) string {
	user, err := h.svc.UserRepo.GetByID(ctx, teleID)
	if err != nil {
		return "vi"
	}
	if user.Language == "" {
		return "vi"
	}
	return user.Language
}

func (h *BotHandler) isUserBanned(ctx context.Context, teleID int64) bool {
	user, err := h.svc.UserRepo.GetByID(ctx, teleID)
	if err != nil {
		return false
	}
	return user.IsBanned
}

// NotifyDeposit sends deposit success notification to a user.
func (h *BotHandler) NotifyDeposit(ctx context.Context, teleID int64, amount decimal.Decimal) {
	lang := h.getUserLang(ctx, teleID)
	user, _ := h.svc.UserRepo.GetByID(ctx, teleID)

	newBalance := "0.00"
	if user != nil {
		newBalance = user.BalanceUSDT.StringFixed(2)
	}

	text := i18n.T(lang, "deposit_success", map[string]interface{}{
		"Amount":     amount.StringFixed(2),
		"NewBalance": newBalance,
	})

	params := tu.Message(tu.ID(teleID), text).WithParseMode(telego.ModeMarkdown)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send deposit notification failed")
	}
}

// handleTxIDInput processes a TxID submitted by the user to verify and credit their deposit.
func (h *BotHandler) handleTxIDInput(ctx context.Context, msg *telego.Message, teleID int64, lang string) {
	defer h.svc.ClearUserState(ctx, teleID)

	txID := strings.TrimSpace(msg.Text)
	if txID == "" || len(txID) < 10 {
		params := tu.Message(tu.ID(msg.Chat.ID), i18n.TSimple(lang, "deposit_txid_invalid"))
		h.bot.SendMessage(ctx, params)
		return
	}

	// Show processing message
	processingMsg := tu.Message(tu.ID(msg.Chat.ID), i18n.TSimple(lang, "deposit_txid_processing"))
	h.bot.SendMessage(ctx, processingMsg)

	amount, err := h.svc.VerifyAndCreditDeposit(ctx, teleID, txID)
	if err != nil {
		errMsg := err.Error()
		var text string
		switch errMsg {
		case "txid_already_used":
			text = i18n.TSimple(lang, "deposit_txid_already_used")
		case "txid_not_found":
			text = i18n.TSimple(lang, "deposit_txid_not_found")
		case "deposit_not_confirmed":
			text = i18n.TSimple(lang, "deposit_txid_not_confirmed")
		case "deposit_processing":
			text = i18n.TSimple(lang, "deposit_txid_processing")
		case "not_usdt":
			text = i18n.TSimple(lang, "deposit_txid_not_usdt")
		case "wrong_address":
			text = i18n.TSimple(lang, "deposit_txid_wrong_address")
		case "binance_not_configured":
			text = i18n.TSimple(lang, "binance_not_configured")
		case "binance_api_error":
			text = i18n.TSimple(lang, "binance_api_error")
		default:
			log.Error().Err(err).Str("txid", txID).Int64("user", teleID).Msg("verify deposit failed")
			text = i18n.TSimple(lang, "error_general")
		}
		params := tu.Message(tu.ID(msg.Chat.ID), text)
		h.bot.SendMessage(ctx, params)
		return
	}

	// Success — notify user
	user, _ := h.svc.UserRepo.GetByID(ctx, teleID)
	newBalance := "0.00"
	if user != nil {
		newBalance = user.BalanceUSDT.StringFixed(2)
	}

	text := i18n.T(lang, "deposit_success", map[string]interface{}{
		"Amount":     amount.StringFixed(2),
		"NewBalance": newBalance,
	})

	params := tu.Message(tu.ID(msg.Chat.ID), text).WithParseMode(telego.ModeMarkdown)
	if _, err := h.bot.SendMessage(ctx, params); err != nil {
		log.Error().Err(err).Msg("send deposit success failed")
	}
}

// StartDailyProductBroadcast runs a background loop that checks every minute
// and sends product updates to users whose local time is 9:00 AM or 9:00 PM.
func (h *BotHandler) StartDailyProductBroadcast(ctx context.Context) {
	log.Info().Msg("product broadcast scheduler started (9 AM & 9 PM per user timezone)")

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.broadcastProducts(ctx)
		case <-ctx.Done():
			log.Info().Msg("product broadcast stopped")
			return
		}
	}
}

// broadcastProducts sends the active product list to users whose local time is 9:00 AM or 9:00 PM.
func (h *BotHandler) broadcastProducts(ctx context.Context) {
	products, err := h.svc.ProductRepo.ListActive(ctx)
	if err != nil {
		log.Error().Err(err).Msg("broadcast: failed to list products")
		return
	}

	users, err := h.svc.UserRepo.ListAllUserLangs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("broadcast: failed to list users")
		return
	}

	if len(users) == 0 {
		return
	}

	nowUTC := time.Now().UTC()
	msgCache := make(map[string]string)
	sentCount := 0

	for _, u := range users {
		tz := u.Timezone
		if tz == "" {
			tz = defaultTimezone
		}

		loc, err := time.LoadLocation(tz)
		if err != nil {
			loc, _ = time.LoadLocation(defaultTimezone)
		}

		userTime := nowUTC.In(loc)
		hour := userTime.Hour()
		minute := userTime.Minute()

		// Only send if user's local time is 9:00 AM (hour=9) or 9:00 PM (hour=21)
		// We check minute=0 because ticker fires every minute
		if (hour != 9 && hour != 21) || minute != 0 {
			continue
		}

		// Deduplicate: check Redis to avoid double-send within the same slot
		slot := fmt.Sprintf("broadcast:%d:%s:%d", u.TeleID, userTime.Format("2006-01-02"), hour)
		locked, _ := h.svc.Redis.SetNX(ctx, slot, "1", 2*time.Hour).Result()
		if !locked {
			continue // already sent for this slot
		}

		lang := u.Language
		if lang == "" {
			lang = "vi"
		}

		text, ok := msgCache[lang]
		if !ok {
			text = h.buildProductListMessage(products, lang)
			msgCache[lang] = text
		}

		params := tu.Message(tu.ID(u.TeleID), text).WithParseMode(telego.ModeMarkdown)
		if _, err := h.bot.SendMessage(ctx, params); err != nil {
			log.Warn().Err(err).Int64("user", u.TeleID).Msg("broadcast: failed to send")
		} else {
			sentCount++
		}

		// Small delay to avoid hitting Telegram rate limits
		time.Sleep(50 * time.Millisecond)
	}

	if sentCount > 0 {
		log.Info().Int("sent", sentCount).Msg("product broadcast completed")
	}
}

// buildProductListMessage builds a formatted product list message for a given language.
func (h *BotHandler) buildProductListMessage(products []models.Product, lang string) string {
	if len(products) == 0 {
		return i18n.TSimple(lang, "daily_no_products")
	}

	var sb strings.Builder
	sb.WriteString(i18n.TSimple(lang, "daily_product_header"))
	sb.WriteString("\n")

	for idx, p := range products {
		line := i18n.T(lang, "daily_product_item", map[string]interface{}{
			"Index": idx + 1,
			"Name":  p.Name(lang),
			"Price": p.PriceUSDT.StringFixed(2),
			"Stock": p.Stock,
		})
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString(i18n.TSimple(lang, "daily_product_footer"))
	return sb.String()
}
