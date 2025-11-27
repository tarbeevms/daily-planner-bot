package bot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"

	"daily-planner/internal/config"
	"daily-planner/internal/model"
	"daily-planner/internal/repository"
	"daily-planner/internal/service"
)

type conversationStage int

const (
	stageNone conversationStage = iota
	stageTitle
	stageDescription
	stageCategory
	stageDeadline
	stageRecurring
	stageRecurringDay
	stageRecurringWindow
)

const (
	cbCompletePrefix = "complete:"
	cbConfirmPrefix  = "confirm:"
	cbCancelPrefix   = "cancel:"
)

const (
	btnSkip             = "Пропустить"
	btnYes              = "Да"
	btnNo               = "Нет"
	btnConfirm          = "Подтвердить"
	btnCancel           = "Отмена"
	btnCancelDialog     = "Отменить"
	noCategory          = "Без раздела"
	noCategoryKey       = "__no_category__"
	iconDefault         = "🟢"
	iconDue             = "🟠"
	iconOverdue         = "🔴"
	iconRecurring       = "🔁"
	menuLabelNewTask    = "Новая задача"
	menuLabelTasks      = "Задачи"
	menuLabelCategories = "Категории"
	menuLabelHelp       = "Помощь"
)

type conversationState struct {
	stage conversationStage
	input service.TaskInput
}

// Bot aggregates Telegram API with services.
type Bot struct {
	api           *tgbotapi.BotAPI
	userRepo      *repository.UserRepository
	categorySvc   *service.CategoryService
	taskSvc       *service.TaskService
	reminderSvc   *service.ReminderService
	config        *config.Config
	conversations map[int64]*conversationState
	confirmations map[int64]uint
	mu            sync.Mutex
}

func New(token string, userRepo *repository.UserRepository, categorySvc *service.CategoryService, taskSvc *service.TaskService, reminderSvc *service.ReminderService, cfg *config.Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("create bot api: %w", err)
	}

	log.Printf("[info] bot authorized on account %s", api.Self.UserName)

	return &Bot{
		api:           api,
		userRepo:      userRepo,
		categorySvc:   categorySvc,
		taskSvc:       taskSvc,
		reminderSvc:   reminderSvc,
		config:        cfg,
		conversations: make(map[int64]*conversationState),
		confirmations: make(map[int64]uint),
	}, nil
}

// Start begins polling updates until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) error {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := b.api.GetUpdatesChan(updateConfig)

	log.Println("[info] start polling updates")

	go func() {
		<-ctx.Done()
		b.api.StopReceivingUpdates()
	}()

	for update := range updates {
		switch {
		case update.CallbackQuery != nil:
			if err := b.handleCallback(ctx, update.CallbackQuery); err != nil {
				log.Printf("handle callback: %v", err)
			}
		case update.Message != nil:
			if update.Message.Chat == nil || !update.Message.Chat.IsPrivate() {
				continue
			}
			if err := b.handleMessage(ctx, update.Message); err != nil {
				log.Printf("handle message: %v", err)
			}
		}
	}

	return nil
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) error {
	if msg.From == nil {
		return nil
	}

	if !msg.IsCommand() && isCancelDialogInput(msg.Text) {
		b.clearConversation(msg.From.ID)
		b.clearConfirmation(msg.From.ID)
		return b.sendText(msg.Chat.ID, "Диалог создания задачи отменен.")
	}

	if !msg.IsCommand() {
		if handled, err := b.handleMenuAlias(msg); handled {
			return err
		}
	}

	if msg.IsCommand() {
		log.Printf("[info] command from %d: /%s %s", msg.From.ID, msg.Command(), msg.CommandArguments())
		return b.handleCommand(ctx, msg)
	}

	if pending, ok := b.getConfirmation(msg.From.ID); ok {
		return b.handleConfirmationResponse(ctx, msg, pending)
	}

	if b.hasConversation(msg.From.ID) {
		log.Printf("[info] conversation step %d from %d", b.getConversation(msg.From.ID).stage, msg.From.ID)
		return b.handleConversation(ctx, msg)
	}

	return b.sendText(msg.Chat.ID, "Не понял сообщение. Используйте /newtask для добавления задачи или /help для списка команд.")
}

func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message) error {
	switch msg.Command() {
	case "start":
		return b.handleStart(msg)
	case "help":
		return b.handleHelp(msg)
	case "newtask":
		return b.startNewTaskConversation(msg)
	case "tasks":
		return b.handleListTasks(msg)
	case "complete":
		return b.handleComplete(msg)
	case "categories":
		return b.handleCategories(msg)
	case "interval":
		return b.handleInterval(msg)
	case "cancel":
		b.clearConversation(msg.From.ID)
		return b.sendText(msg.Chat.ID, "Диалог создания задачи отменен.")
	default:
		return b.sendText(msg.Chat.ID, "Команда не поддерживается. Посмотрите /help.")
	}
}

func (b *Bot) handleStart(msg *tgbotapi.Message) error {
	if _, err := b.ensureUser(msg.From); err != nil {
		return err
	}
	text := "Привет! Я бот-ежедневник. Доступные команды:\n" +
		"/newtask — создать новую задачу\n" +
		"/tasks — показать активные задачи\n" +
		"/complete <id> — отметить задачу выполненной\n" +
		"/categories — показать разделы\n" +
		"/help — справка\n" +
		"/cancel — отменить создание задачи"
	return b.sendText(msg.Chat.ID, text)
}

func (b *Bot) handleHelp(msg *tgbotapi.Message) error {
	text := "Команды:\n" +
		"/newtask — добавить задачу через диалог\n" +
		"/tasks — список активных задач\n" +
		"/complete <id> — отметить задачу выполненной (для регулярных задач фиксирует выполнение в текущем окне)\n" +
		"/categories — список ваших разделов\n" +
		"/interval <часы> — настроить периодичность напоминаний (по умолчанию 5 часов)\n" +
		"/cancel — отменить текущий диалог"
	return b.sendText(msg.Chat.ID, text)
}

func (b *Bot) startNewTaskConversation(msg *tgbotapi.Message) error {
	if _, err := b.ensureUser(msg.From); err != nil {
		return err
	}
	log.Printf("[info] start new task conversation user=%d", msg.From.ID)
	b.setConversation(msg.From.ID, &conversationState{stage: stageTitle})
	return b.sendWithReplyMarkup(msg.Chat.ID, "Введите название задачи:", cancelKeyboard())
}

func (b *Bot) handleConversation(ctx context.Context, msg *tgbotapi.Message) error {
	state := b.getConversation(msg.From.ID)
	if state == nil {
		return nil
	}

	text := strings.TrimSpace(msg.Text)
	switch state.stage {
	case stageTitle:
		state.input.Title = text
		state.stage = stageDescription
		return b.sendWithReplyMarkup(msg.Chat.ID, "Добавьте описание:", skipKeyboard())
	case stageDescription:
		if !isSkipInput(text) {
			state.input.Description = text
		}
		state.stage = stageCategory
		return b.sendWithReplyMarkup(msg.Chat.ID, "Укажите раздел (например, Работа, Здоровье):", categoryKeyboard())
	case stageCategory:
		if !isSkipInput(text) {
			state.input.Category = text
		}
		state.stage = stageDeadline
		return b.sendWithReplyMarkup(msg.Chat.ID, "Введите дедлайн в формате ГГГГ-ММ-ДД:", skipKeyboard())
	case stageDeadline:
		if !isSkipInput(text) {
			parsed, err := time.Parse("2006-01-02", text)
			if err != nil {
				return b.sendWithReplyMarkup(msg.Chat.ID, "Не удалось разобрать дату. Используйте формат ГГГГ-ММ-ДД или нажмите \"Пропустить\".", skipKeyboard())
			}
			state.input.Deadline = &parsed
		}
		state.stage = stageRecurring
		return b.sendWithReplyMarkup(msg.Chat.ID, "Сделать задачу ежемесячной?", yesNoKeyboard())
	case stageRecurring:
		lower := strings.ToLower(text)
		if lower == "да" || lower == "yes" || lower == "y" {
			state.input.IsRecurring = true
			state.stage = stageRecurringDay
			return b.sendWithReplyMarkup(msg.Chat.ID, "Укажите день месяца (1-31), когда задача должна быть сделана:", tgbotapi.NewRemoveKeyboard(true))
		}
		if lower == "нет" || lower == "no" || lower == "n" || lower == "-" {
			state.input.IsRecurring = false
			err := b.finishTaskCreation(ctx, msg.From, state.input, msg.Chat.ID)
			b.clearConversation(msg.From.ID)
			return err
		}
		return b.sendWithReplyMarkup(msg.Chat.ID, "Ответьте \"Да\" или \"Нет\".", yesNoKeyboard())
	case stageRecurringDay:
		day, err := strconv.Atoi(text)
		if err != nil || day < 1 || day > 31 {
			return b.sendText(msg.Chat.ID, "День месяца должен быть числом от 1 до 31.")
		}
		state.input.RecurDay = day
		state.stage = stageRecurringWindow
		return b.sendWithReplyMarkup(msg.Chat.ID, "Сколько дней окно для выполнения вокруг даты? (например, 2):", tgbotapi.NewRemoveKeyboard(true))
	case stageRecurringWindow:
		window, err := strconv.Atoi(text)
		if err != nil || window < 0 || window > 14 {
			return b.sendText(msg.Chat.ID, "Окно должно быть числом от 0 до 14.")
		}
		state.input.RecurWindow = window
		err = b.finishTaskCreation(ctx, msg.From, state.input, msg.Chat.ID)
		b.clearConversation(msg.From.ID)
		return err
	default:
		b.clearConversation(msg.From.ID)
		return b.sendText(msg.Chat.ID, "Диалог сброшен. Начните заново через /newtask.")
	}
}

func (b *Bot) finishTaskCreation(ctx context.Context, from *tgbotapi.User, input service.TaskInput, chatID int64) error {
	user, err := b.ensureUser(from)
	if err != nil {
		return err
	}

	task, err := b.taskSvc.CreateTask(user, input)
	if err != nil {
		return b.sendText(chatID, fmt.Sprintf("Не удалось создать задачу: %v", err))
	}

	log.Printf("[info] task created id=%d user=%d recurring=%t", task.ID, user.ID, task.IsRecurring)

	var summary strings.Builder
	summary.WriteString("✅ Задача создана\n")
	summary.WriteString(fmt.Sprintf("ID: %d\n", task.ID))
	summary.WriteString(fmt.Sprintf("Название: %s\n", task.Title))
	if task.Description != "" {
		summary.WriteString(fmt.Sprintf("Описание: %s\n", task.Description))
	}
	if task.Deadline != nil {
		summary.WriteString(fmt.Sprintf("Дедлайн: %s\n", task.Deadline.Format("2006-01-02")))
	}
	if task.IsRecurring {
		summary.WriteString(fmt.Sprintf("Ежемесячно: день %d, окно ±%d дней\n", task.RecurDay, task.RecurWindow))
	}

	msg := tgbotapi.NewMessage(chatID, summary.String())
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	if _, err := b.api.Send(msg); err != nil {
		return err
	}
	return b.sendTaskList(chatID, user)
}

func (b *Bot) handleListTasks(msg *tgbotapi.Message) error {
	user, err := b.ensureUser(msg.From)
	if err != nil {
		return err
	}

	log.Printf("[info] list tasks for user=%d", user.ID)
	return b.sendTaskList(msg.Chat.ID, user)
}

func (b *Bot) handleComplete(msg *tgbotapi.Message) error {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		return b.sendText(msg.Chat.ID, "Укажите ID задачи: /complete 12")
	}

	taskID64, err := strconv.ParseUint(args, 10, 64)
	if err != nil {
		return b.sendText(msg.Chat.ID, "ID должен быть числом.")
	}

	user, err := b.ensureUser(msg.From)
	if err != nil {
		return err
	}

	task, err := b.taskSvc.CompleteTask(user, uint(taskID64), time.Now())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return b.sendText(msg.Chat.ID, "Задача не найдена.")
		}
		return b.sendText(msg.Chat.ID, fmt.Sprintf("Ошибка: %v", err))
	}

	if task.IsRecurring {
		return b.sendText(msg.Chat.ID, fmt.Sprintf("Отметил выполнение регулярной задачи \"%s\" для текущего окна.", task.Title))
	}

	return b.sendText(msg.Chat.ID, fmt.Sprintf("Задача \"%s\" отмечена как выполненная.", task.Title))
}

func (b *Bot) handleCategories(msg *tgbotapi.Message) error {
	user, err := b.ensureUser(msg.From)
	if err != nil {
		return err
	}
	categories, err := b.categorySvc.List(user)
	if err != nil {
		return b.sendText(msg.Chat.ID, fmt.Sprintf("Не удалось получить разделы: %v", err))
	}
	if len(categories) == 0 {
		return b.sendText(msg.Chat.ID, "Разделы пока не созданы. Они добавляются автоматически при создании задач.")
	}
	var builder strings.Builder
	builder.WriteString("Ваши разделы:\n")
	for _, cat := range categories {
		builder.WriteString(fmt.Sprintf("• %s\n", cat.Name))
	}
	return b.sendText(msg.Chat.ID, builder.String())
}

func (b *Bot) handleConfirmationResponse(ctx context.Context, msg *tgbotapi.Message, taskID uint) error {
	text := strings.TrimSpace(msg.Text)
	switch {
	case isConfirmInput(text):
		b.clearConfirmation(msg.From.ID)
		return b.completeTaskAndRefresh(ctx, msg.Chat.ID, msg.From, taskID)
	case isCancelInput(text):
		b.clearConfirmation(msg.From.ID)
		return b.sendMenuPlaceholder(msg.Chat.ID)
	default:
		return b.sendWithReplyMarkup(msg.Chat.ID, "Подтвердите или отмените действие.", confirmKeyboard())
	}
}

// SendDailyReports sends a summary to every known user.
func (b *Bot) SendDailyReports(ctx context.Context) error {
	users, err := b.userRepo.ListAll()
	if err != nil {
		return err
	}
	now := time.Now()
	for _, user := range users {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		text, err := b.reminderSvc.DailySummary(user, now)
		if err != nil {
			log.Printf("build summary for user %d: %v", user.TelegramID, err)
			continue
		}
		if err := b.sendText(user.TelegramID, text); err != nil {
			log.Printf("send summary to %d: %v", user.TelegramID, err)
		}
	}
	return nil
}

func (b *Bot) handleInterval(msg *tgbotapi.Message) error {
	if msg.From == nil {
		return nil
	}
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		current := "5 часов"
		if b.config != nil && b.config.ReportInterval > 0 {
			current = b.config.ReportInterval.String()
		}
		return b.sendText(msg.Chat.ID, fmt.Sprintf("Текущий интервал напоминаний: %s. Укажите число часов, например: /interval 4", current))
	}
	hours, err := strconv.Atoi(args)
	if err != nil || hours <= 0 {
		return b.sendText(msg.Chat.ID, "Интервал должен быть положительным числом часов, например /interval 6")
	}
	b.mu.Lock()
	b.config.ReportInterval = time.Duration(hours) * time.Hour
	b.mu.Unlock()
	return b.sendText(msg.Chat.ID, fmt.Sprintf("Интервал напоминаний обновлен: каждые %d часов.", hours))
}

func (b *Bot) ensureUser(from *tgbotapi.User) (*model.User, error) {
	return b.userRepo.UpsertFromTelegram(from.ID, from.FirstName, from.LastName, from.UserName)
}

func (b *Bot) sendText(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = mainMenuKeyboard()
	_, err := b.api.Send(msg)
	return err
}

func (b *Bot) sendTextWithRemove(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	if _, err := b.api.Send(msg); err != nil {
		return err
	}
	return b.sendMenuPlaceholder(chatID)
}

func (b *Bot) sendWithReplyMarkup(chatID int64, text string, markup interface{}) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup
	_, err := b.api.Send(msg)
	return err
}

func (b *Bot) sendMenuPlaceholder(chatID int64) error {
	msg := tgbotapi.NewMessage(chatID, "Меню:")
	msg.ReplyMarkup = mainMenuKeyboard()
	_, err := b.api.Send(msg)
	return err
}

func (b *Bot) getConfirmation(userID int64) (uint, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	taskID, ok := b.confirmations[userID]
	return taskID, ok
}

func (b *Bot) setConfirmation(userID int64, taskID uint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.confirmations[userID] = taskID
}

func (b *Bot) clearConfirmation(userID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.confirmations, userID)
}

func (b *Bot) setConversation(userID int64, state *conversationState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.conversations[userID] = state
}

func (b *Bot) getConversation(userID int64) *conversationState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.conversations[userID]
}

func (b *Bot) hasConversation(userID int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.conversations[userID]
	return ok
}

func (b *Bot) clearConversation(userID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.conversations, userID)
}

func (b *Bot) sendTaskList(chatID int64, user *model.User) error {
	tasks, err := b.taskSvc.ListActive(user)
	if err != nil {
		return b.sendText(chatID, fmt.Sprintf("Не удалось получить задачи: %v", err))
	}

	categories, _ := b.categorySvc.List(user)
	catNames := make(map[uint]string)
	for _, cat := range categories {
		catNames[cat.ID] = cat.Name
	}

	now := time.Now()
	type categoryGroup struct {
		Name  string
		Tasks []model.Task
	}

	groups := make(map[string]*categoryGroup)
	order := make([]string, 0, len(tasks))

	for _, task := range tasks {
		if !task.IsRecurring && task.IsCompleted {
			continue
		}
		key, display := normalizedCategory(task.CategoryID, catNames)
		group, ok := groups[key]
		if !ok {
			group = &categoryGroup{Name: display}
			groups[key] = group
			order = append(order, key)
		}
		groups[key].Tasks = append(groups[key].Tasks, task)
	}

	if len(groups) == 0 {
		return b.sendText(chatID, "Нет активных задач. Создайте новую через /newtask.")
	}

	sort.Slice(order, func(i, j int) bool {
		if order[i] == noCategoryKey {
			return false
		}
		if order[j] == noCategoryKey {
			return true
		}
		return strings.Compare(groups[order[i]].Name, groups[order[j]].Name) < 0
	})

	var builder strings.Builder
	builder.WriteString("<b>Активные задачи:</b>\n\n")

	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, key := range order {
		section := groups[key]
		sort.SliceStable(section.Tasks, func(i, j int) bool {
			a := section.Tasks[i]
			b := section.Tasks[j]
			if a.Deadline != nil && b.Deadline != nil {
				if !a.Deadline.Equal(*b.Deadline) {
					return a.Deadline.Before(*b.Deadline)
				}
			} else if a.Deadline != nil {
				return true
			} else if b.Deadline != nil {
				return false
			}
			if a.IsRecurring != b.IsRecurring {
				return !a.IsRecurring && b.IsRecurring
			}
			return a.ID < b.ID
		})

		builder.WriteString(fmt.Sprintf("%s\n", section.Name))
		for _, task := range section.Tasks {
			if task.IsRecurring {
				builder.WriteString(formatRecurringTask(task, now))
				buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("✅ [%d] %s", task.ID, shortTitle(normalizeTitle(task.Title), 20)), fmt.Sprintf("%s%d", cbCompletePrefix, task.ID)),
				})
			} else {
				builder.WriteString(formatTask(task, now))
				buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("✅ [%d] %s", task.ID, shortTitle(normalizeTitle(task.Title), 24)), fmt.Sprintf("%s%d", cbCompletePrefix, task.ID)),
				})
			}
		}
		builder.WriteByte('\n')
	}

	msg := tgbotapi.NewMessage(chatID, strings.TrimSpace(builder.String()))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	msg.ParseMode = "HTML"
	_, err = b.api.Send(msg)
	return err
}

func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) error {
	if cb == nil || cb.From == nil || cb.Message == nil {
		return nil
	}

	data := cb.Data

	switch {
	case strings.HasPrefix(data, cbCompletePrefix):
		log.Printf("[info] callback complete request user=%d task=%s", cb.From.ID, strings.TrimPrefix(data, cbCompletePrefix))
		if _, err := b.api.Request(tgbotapi.NewCallback(cb.ID, "")); err != nil {
			log.Printf("callback ack: %v", err)
		}
		taskID, err := parseTaskID(data, cbCompletePrefix)
		if err != nil {
			return nil
		}
		return b.askCompleteConfirmation(ctx, cb.Message.Chat.ID, cb.From, taskID)
	case strings.HasPrefix(data, cbConfirmPrefix):
		log.Printf("[info] callback confirm complete user=%d task=%s", cb.From.ID, strings.TrimPrefix(data, cbConfirmPrefix))
		if _, err := b.api.Request(tgbotapi.NewCallback(cb.ID, "")); err != nil {
			log.Printf("callback ack: %v", err)
		}
		taskID, err := parseTaskID(data, cbConfirmPrefix)
		if err != nil {
			return nil
		}
		return b.completeTaskAndRefresh(ctx, cb.Message.Chat.ID, cb.From, taskID)
	case strings.HasPrefix(data, cbCancelPrefix):
		log.Printf("[info] callback cancel complete user=%d task=%s", cb.From.ID, strings.TrimPrefix(data, cbCancelPrefix))
		if _, err := b.api.Request(tgbotapi.NewCallback(cb.ID, "")); err != nil {
			log.Printf("callback ack: %v", err)
		}
		return nil
	default:
		if _, err := b.api.Request(tgbotapi.NewCallback(cb.ID, "")); err != nil {
			log.Printf("callback ack: %v", err)
		}
		return nil
	}
}

func (b *Bot) askCompleteConfirmation(ctx context.Context, chatID int64, from *tgbotapi.User, taskID uint) error {
	user, err := b.ensureUser(from)
	if err != nil {
		return err
	}

	task, err := b.taskSvc.GetTask(user, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return b.sendText(chatID, "Задача не найдена.")
		}
		return err
	}

	if task.IsRecurring {
		if isRecurringDoneInWindow(*task, time.Now()) {
			return b.sendText(chatID, "Задача уже отмечена выполненной в текущем окне.")
		}
	} else if task.IsCompleted {
		return b.sendText(chatID, "Задача уже завершена.")
	}

	text := fmt.Sprintf("Отметить задачу \"%s\" (#%d) как выполненную?", task.Title, task.ID)
	b.setConfirmation(from.ID, task.ID)
	return b.sendWithReplyMarkup(chatID, text, confirmKeyboard())
}

func (b *Bot) completeTaskAndRefresh(ctx context.Context, chatID int64, from *tgbotapi.User, taskID uint) error {
	user, err := b.ensureUser(from)
	if err != nil {
		return err
	}

	task, err := b.taskSvc.GetTask(user, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return b.sendTextWithRemove(chatID, "Задача не найдена или уже удалена.")
		}
		return b.sendTextWithRemove(chatID, fmt.Sprintf("Ошибка: %v", err))
	}

	now := time.Now()
	if task.IsRecurring && isRecurringDoneInWindow(*task, now) {
		return b.sendTextWithRemove(chatID, "Задача уже отмечена выполненной в текущем окне.")
	}
	if !task.IsRecurring && task.IsCompleted {
		return b.sendTextWithRemove(chatID, "Задача уже завершена.")
	}

	task, err = b.taskSvc.CompleteTask(user, taskID, now)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return b.sendTextWithRemove(chatID, "Задача не найдена или уже удалена.")
		}
		return b.sendTextWithRemove(chatID, fmt.Sprintf("Ошибка: %v", err))
	}

	var info string
	if task.IsRecurring {
		info = fmt.Sprintf("Отметил регулярную задачу \"%s\" для текущего окна.", task.Title)
	} else {
		info = fmt.Sprintf("Задача \"%s\" отмечена как выполненная.", task.Title)
	}
	log.Printf("[info] task completed id=%d user=%d recurring=%t", task.ID, user.ID, task.IsRecurring)
	if err := b.sendTextWithRemove(chatID, info); err != nil {
		return err
	}

	return b.sendTaskList(chatID, user)
}

func parseTaskID(data, prefix string) (uint, error) {
	raw := strings.TrimPrefix(data, prefix)
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(value), nil
}

func shortTitle(title string, maxLen int) string {
	runes := []rune(title)
	if len(runes) <= maxLen {
		return title
	}
	return string(runes[:maxLen]) + "…"
}

func (b *Bot) handleMenuAlias(msg *tgbotapi.Message) (bool, error) {
	text := strings.TrimSpace(strings.ToLower(msg.Text))
	switch text {
	case strings.ToLower(menuLabelNewTask):
		return true, b.startNewTaskConversation(msg)
	case strings.ToLower(menuLabelTasks):
		return true, b.handleListTasks(msg)
	case strings.ToLower(menuLabelCategories):
		return true, b.handleCategories(msg)
	case strings.ToLower(menuLabelHelp):
		return true, b.handleHelp(msg)
	default:
		return false, nil
	}
}

func confirmKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnConfirm),
			tgbotapi.NewKeyboardButton(btnCancel),
			tgbotapi.NewKeyboardButton(btnCancelDialog),
		),
	)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	return kb
}

func mainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(menuLabelNewTask),
			tgbotapi.NewKeyboardButton(menuLabelTasks),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(menuLabelCategories),
			tgbotapi.NewKeyboardButton(menuLabelHelp),
		),
	)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = false
	return kb
}

func cancelKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnCancelDialog),
		),
	)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	return kb
}

func skipKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnSkip),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnCancelDialog),
		),
	)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	return kb
}

func yesNoKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnYes),
			tgbotapi.NewKeyboardButton(btnNo),
			tgbotapi.NewKeyboardButton(btnCancelDialog),
		),
	)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	return kb
}

func categoryKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Учеба"),
			tgbotapi.NewKeyboardButton("Здоровье"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Работа"),
			tgbotapi.NewKeyboardButton("Покупки"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnSkip),
			tgbotapi.NewKeyboardButton(btnCancelDialog),
		),
	)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	return kb
}

func isSkipInput(text string) bool {
	value := strings.TrimSpace(strings.ToLower(text))
	return value == "-" || value == strings.ToLower(btnSkip) || value == "skip"
}

func isConfirmInput(text string) bool {
	value := strings.TrimSpace(strings.ToLower(text))
	return value == strings.ToLower(btnConfirm)
}

func isCancelInput(text string) bool {
	value := strings.TrimSpace(strings.ToLower(text))
	return value == strings.ToLower(btnCancel)
}

func isCancelDialogInput(text string) bool {
	value := strings.TrimSpace(strings.ToLower(text))
	return value == strings.ToLower(btnCancelDialog)
}

func isRecurringDoneInWindow(task model.Task, now time.Time) bool {
	if !task.IsRecurring || task.LastCompletedAt == nil {
		return false
	}

	year, month, _ := now.Date()
	dueDay := task.RecurDay
	endOfMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, now.Location()).Day()
	if dueDay > endOfMonth {
		dueDay = endOfMonth
	}

	dueDate := time.Date(year, month, dueDay, 0, 0, 0, 0, now.Location())
	window := time.Duration(task.RecurWindow) * 24 * time.Hour
	start := dueDate.Add(-window)
	end := dueDate.Add(window)

	last := task.LastCompletedAt.In(now.Location())
	if last.Before(start) || last.After(end) {
		return false
	}
	if last.Month() != now.Month() || last.Year() != now.Year() {
		return false
	}
	return true
}

func normalizeTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func categoryLabel(name string) string {
	base := strings.TrimSpace(name)
	lower := strings.ToLower(base)
	var icon string
	switch lower {
	case "учеба":
		icon = "🎓"
	case "здоровье":
		icon = "💪"
	case "работа":
		icon = "💼"
	case "покупки":
		icon = "🛒"
	case strings.ToLower(noCategory):
		icon = "📁"
	default:
		icon = "📂"
	}
	return fmt.Sprintf("%s %s", icon, escape(normalizeTitle(base)))
}

func escape(s string) string {
	return html.EscapeString(s)
}

func normalizedCategory(categoryID *uint, catNames map[uint]string) (string, string) {
	if categoryID == nil {
		return noCategoryKey, categoryLabel(noCategory)
	}
	if name, ok := catNames[*categoryID]; ok {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return noCategoryKey, categoryLabel(noCategory)
		}
		return strings.ToLower(trimmed), categoryLabel(trimmed)
	}
	return noCategoryKey, categoryLabel(noCategory)
}

func formatTask(task model.Task, now time.Time) string {
	var b strings.Builder
	icon := iconDefault
	if task.Deadline != nil {
		d := task.Deadline.In(now.Location())
		if now.After(d) {
			icon = iconOverdue
		} else if d.Sub(now) <= 48*time.Hour {
			icon = iconDue
		}
	}
	b.WriteString(fmt.Sprintf("%s [%d] %s\n", icon, task.ID, escape(normalizeTitle(task.Title))))
	if task.Deadline != nil {
		d := task.Deadline.In(now.Location())
		if now.After(d) {
			b.WriteString(fmt.Sprintf("  ⚠️ дедлайн: %s (просрочено)\n", d.Format("2006-01-02")))
		} else {
			b.WriteString(fmt.Sprintf("  📅 дедлайн: %s\n", d.Format("2006-01-02")))
		}
	}
	if task.Description != "" {
		b.WriteString(fmt.Sprintf("  📝 %s\n", escape(task.Description)))
	}
	b.WriteByte('\n')
	return b.String()
}

func formatRecurringTask(task model.Task, now time.Time) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s [%d] %s\n", iconRecurring, task.ID, escape(normalizeTitle(task.Title))))

	year, month, _ := now.Date()
	dueDay := task.RecurDay
	endOfMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, now.Location()).Day()
	if dueDay > endOfMonth {
		dueDay = endOfMonth
	}
	dueDate := time.Date(year, month, dueDay, 0, 0, 0, 0, now.Location())

	b.WriteString(fmt.Sprintf("  📅 дата выполнения: %s (окно ±%d дн.)\n", dueDate.Format("2006-01-02"), task.RecurWindow))
	if task.LastCompletedAt != nil {
		b.WriteString(fmt.Sprintf("  🔄 последнее выполнение: %s\n", task.LastCompletedAt.In(now.Location()).Format("2006-01-02")))
	} else {
		b.WriteString("  🔄 еще не выполнялась\n")
	}
	b.WriteByte('\n')
	return b.String()
}
