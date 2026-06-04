package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/chtisgit/vibegang/pkg/config"
	"github.com/chtisgit/vibegang/pkg/db"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func runMailTUI(cfgPath string) {
	// Apply Tokyo Night Theme
	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    tcell.NewHexColor(0x1a1b26), // Deep rich blue-grey
		ContrastBackgroundColor:     tcell.NewHexColor(0x24283b), // Tokyo slate
		MoreContrastBackgroundColor: tcell.NewHexColor(0x414868), // Mid grey
		BorderColor:                 tcell.NewHexColor(0x7aa2f7), // Bright aesthetic blue
		TitleColor:                  tcell.NewHexColor(0xbb9af7), // Purple/Magenta accent
		GraphicsColor:               tcell.NewHexColor(0x7aa2f7),
		PrimaryTextColor:            tcell.NewHexColor(0xc0caf5), // Light lavender grey
		SecondaryTextColor:          tcell.NewHexColor(0xe0af68), // Warm yellow-gold
		TertiaryTextColor:           tcell.NewHexColor(0x9ece6a), // Lime green
		InverseTextColor:            tcell.NewHexColor(0x1a1b26),
		ContrastSecondaryTextColor:  tcell.NewHexColor(0x565f89), // Soft blue-grey
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Printf("Warning: Failed to load config from %s: %v. Using defaults.", cfgPath, err)
		cfg = &config.Config{
			UserEmail: "user@local",
		}
	}

	dbConnStr := getDBConnStr()
	dbClient, err := db.NewDB(dbConnStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close()

	app := tview.NewApplication()
	pages := tview.NewPages()

	// Recipient lists for composer
	var options []string
	var optionLabels []string
	if cfg.UserEmail != "" {
		options = append(options, cfg.UserEmail)
		optionLabels = append(optionLabels, fmt.Sprintf("%s (User)", cfg.UserEmail))
	}
	for _, agent := range cfg.Agents {
		options = append(options, agent.Email)
		optionLabels = append(optionLabels, fmt.Sprintf("%s (%s - %s)", agent.Email, agent.Name, agent.Role))
	}
	options = append(options, "")
	optionLabels = append(optionLabels, "<Custom Email>")

	// Forward declaration of helpers
	var refreshInbox func()
	var showComposer func(replyTo, replySubject, replyBody string, focusBody bool)

	// --- INBOX VIEW ---
	inboxTable := tview.NewTable().SetSelectable(true, false)
	inboxTable.SetBorder(true).
		SetTitle(fmt.Sprintf(" Inbox for %s ", cfg.UserEmail)).
		SetTitleAlign(tview.AlignLeft)

	// Help Bar
	helpText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	helpText.SetText(" [yellow]Enter[white] Read Mail  [yellow]N[white] New Mail  [yellow]R[white] Refresh  [yellow]Esc[white] Exit ")

	inboxFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(inboxTable, 0, 1, true).
		AddItem(helpText, 1, 0, false)

	// Track currently listed mails
	var currentMails []db.MailSummary

	refreshInbox = func() {
		inboxTable.Clear()
		// Headers
		headers := []string{"Status  ", "ID      ", "From                      ", "Subject", "Date"}
		for col, h := range headers {
			cell := tview.NewTableCell(h).
				SetTextColor(tcell.NewHexColor(0xbb9af7)).
				SetSelectable(false).
				SetAttributes(tcell.AttrBold)
			if col == 3 {
				cell.SetExpansion(1)
			}
			inboxTable.SetCell(0, col, cell)
		}

		mails, err := dbClient.GetAllSummary(cfg.UserEmail)
		if err != nil {
			inboxTable.SetCell(1, 0, tview.NewTableCell(fmt.Sprintf("Error loading inbox: %v", err)))
			return
		}
		currentMails = mails

		if len(mails) == 0 {
			inboxTable.SetCell(1, 0, tview.NewTableCell("No emails found.").SetTextColor(tcell.ColorDarkGray))
			return
		}

		for row, mail := range mails {
			statusCell := tview.NewTableCell("   ")
			if !mail.IsRead {
				statusCell.SetText(" ● ").SetTextColor(tcell.NewHexColor(0xe0af68)) // Bold yellow bullet for unread
			}

			idStr := fmt.Sprintf("%d      ", mail.ID)
			fromStr := fmt.Sprintf("%-26s", mail.From)
			subjectStr := mail.Subject
			dateStr := mail.Timestamp.Local().Format("2006-01-02 15:04:05")

			inboxTable.SetCell(row+1, 0, statusCell)
			inboxTable.SetCell(row+1, 1, tview.NewTableCell(idStr).SetTextColor(tcell.NewHexColor(0x7aa2f7)))
			inboxTable.SetCell(row+1, 2, tview.NewTableCell(fromStr).SetTextColor(tcell.NewHexColor(0xc0caf5)))

			subjectCell := tview.NewTableCell(subjectStr).SetTextColor(tcell.NewHexColor(0xc0caf5)).SetExpansion(1)
			inboxTable.SetCell(row+1, 3, subjectCell)

			inboxTable.SetCell(row+1, 4, tview.NewTableCell(dateStr).SetTextColor(tcell.NewHexColor(0x565f89)))
		}
		inboxTable.Select(1, 0)
	}

	// --- VIEW MAIL VIEW ---
	mailDetailText := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	mailDetailText.SetBorder(true).SetTitleAlign(tview.AlignLeft)

	var activeMail *db.Mail

	mailDetailHelp := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	mailDetailHelp.SetText(" [yellow]R[white] Reply  [yellow]B[white] Back to Inbox  [yellow]Esc[white] Exit ")

	mailDetailFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(mailDetailText, 0, 1, true).
		AddItem(mailDetailHelp, 1, 0, false)

	showMail := func(summary db.MailSummary) {
		mail, err := dbClient.ReadMail(cfg.UserEmail, summary.ID)
		if err != nil {
			// Show error modal
			modal := tview.NewModal().
				SetText(fmt.Sprintf("Failed to read email: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.RemovePage("error")
				})
			pages.AddPage("error", modal, true, true)
			return
		}
		activeMail = mail

		mailDetailText.Clear()
		mailDetailText.SetTitle(fmt.Sprintf(" Email Details (ID: %d) ", mail.ID))

		headersStr := fmt.Sprintf(
			"[purple]From:[white]    %s\n[purple]To:[white]      %s\n[purple]Subject:[white] %s\n[purple]Date:[white]    %s\n\n[blue]Body:[white]\n\n%s",
			mail.From, mail.To, mail.Subject, mail.Timestamp.Local().Format("2006-01-02 15:04:05"), mail.Body,
		)
		mailDetailText.SetText(headersStr)
		pages.SwitchToPage("view-mail")
	}

	// Selection callback on table
	inboxTable.SetSelectedFunc(func(row, column int) {
		if row > 0 && row-1 < len(currentMails) {
			showMail(currentMails[row-1])
		}
	})

	// --- COMPOSER VIEW ---
	showComposer = func(replyTo, replySubject, replyBody string, focusBody bool) {
		form := tview.NewForm()
		form.SetBorder(true).SetTitle(" Send Email ").SetTitleAlign(tview.AlignLeft)

		fromInput := tview.NewInputField().
			SetLabel("From:").
			SetText(cfg.UserEmail).
			SetFieldWidth(40)
		form.AddFormItem(fromInput)

		customToInput := tview.NewInputField().
			SetLabel("Custom To:").
			SetFieldWidth(40)

		toDropdown := tview.NewDropDown().
			SetLabel("To Agent:").
			SetOptions(optionLabels, func(text string, index int) {
				if index >= 0 && index < len(options) {
					selected := options[index]
					if selected == "" {
						customToInput.SetDisabled(false)
					} else {
						customToInput.SetDisabled(true)
						customToInput.SetText(selected)
					}
				}
			})

		toIndex := 0
		if replyTo != "" {
			found := false
			for idx, opt := range options {
				if opt == replyTo {
					toIndex = idx
					found = true
					break
				}
			}
			if !found {
				toIndex = len(options) - 1
				customToInput.SetText(replyTo)
			}
		}
		toDropdown.SetCurrentOption(toIndex)
		if toIndex != len(options)-1 {
			customToInput.SetDisabled(true)
			customToInput.SetText(options[toIndex])
		}

		form.AddFormItem(toDropdown)
		form.AddFormItem(customToInput)

		subjectInput := tview.NewInputField().
			SetLabel("Subject:").
			SetText(replySubject).
			SetFieldWidth(60)
		form.AddFormItem(subjectInput)

		bodyInput := tview.NewTextArea().
			SetLabel("Body:")
		bodyInput.SetText(replyBody, !focusBody)
		form.AddFormItem(bodyInput)

		if focusBody {
			form.SetFocus(4)
		}

		form.AddButton("Send", func() {
			fromVal := fromInput.GetText()
			toVal := customToInput.GetText()
			subjectVal := subjectInput.GetText()
			bodyVal := bodyInput.GetText()

			if fromVal == "" || toVal == "" || subjectVal == "" || bodyVal == "" {
				modal := tview.NewModal().
					SetText("All fields are required.").
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						pages.RemovePage("composer-error")
					})
				pages.AddPage("composer-error", modal, true, true)
				return
			}

			if err := dbClient.SendMail(fromVal, toVal, subjectVal, bodyVal); err != nil {
				modal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to send: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						pages.RemovePage("composer-error")
					})
				pages.AddPage("composer-error", modal, true, true)
				return
			}

			pages.RemovePage("composer")
			refreshInbox()
			pages.SwitchToPage("inbox")
		})

		form.AddButton("Cancel", func() {
			modal := tview.NewModal().
				SetText("Discard email?").
				AddButtons([]string{"Yes", "No"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.RemovePage("confirm-discard")
					if buttonLabel == "Yes" {
						pages.RemovePage("composer")
						refreshInbox()
						pages.SwitchToPage("inbox")
					}
				})
			pages.AddPage("confirm-discard", modal, true, true)
		})

		// Layout grid
		grid := tview.NewGrid().
			SetRows(0, 22, 0).
			SetColumns(0, 80, 0).
			AddItem(form, 1, 1, 1, 1, 0, 0, true)

		pages.AddPage("composer", grid, true, true)
		pages.SwitchToPage("composer")
	}

	// Key Captures on Inbox Table
	inboxFlex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'r', 'R':
				refreshInbox()
				return nil
			case 'n', 'N':
				showComposer("", "", "", false)
				return nil
			}
		} else if event.Key() == tcell.KeyEscape {
			app.Stop()
			return nil
		}
		return event
	})

	// Key Captures on Mail Details
	mailDetailFlex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'b', 'B':
				refreshInbox()
				pages.SwitchToPage("inbox")
				return nil
			case 'r', 'R':
				if activeMail != nil {
					subj := activeMail.Subject
					if !strings.HasPrefix(strings.ToLower(subj), "re:") {
						subj = "Re: " + subj
					}
					quotedBody := fmt.Sprintf("\n\n--- On %s, %s wrote:\n> %s",
						activeMail.Timestamp.Local().Format("2006-01-02 15:04:05"),
						activeMail.From,
						strings.ReplaceAll(activeMail.Body, "\n", "\n> "),
					)
					showComposer(activeMail.From, subj, quotedBody, true)
				}
				return nil
			}
		} else if event.Key() == tcell.KeyEscape {
			app.Stop()
			return nil
		}
		return event
	})

	pages.AddPage("inbox", inboxFlex, true, true)
	pages.AddPage("view-mail", mailDetailFlex, true, false)

	refreshInbox()

	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		log.Fatalf("TUI Error: %v", err)
	}
}
