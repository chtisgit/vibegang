package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chtisgit/vibegang/pkg/config"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"
)

func generateEmail(name string, domain string, existing map[string]bool) string {
	parts := strings.Fields(strings.ToLower(name))
	base := "agent"
	if len(parts) > 0 {
		base = strings.Join(parts, ".")
	}

	if domain == "" {
		domain = "vibegang.local"
	}

	email := base + "@" + domain
	counter := 1
	for existing[email] {
		email = fmt.Sprintf("%s%d@%s", base, counter, domain)
		counter++
	}
	return email
}

func runSetup() {
	// Apply Tokyo Night Styling for maximum premium vibe
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

	app := tview.NewApplication()
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" Configuration Panel ").SetTitleAlign(tview.AlignLeft)

	previewView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	previewView.SetBorder(true).SetTitle(" Dynamic Team Roster ").SetTitleAlign(tview.AlignLeft)

	usedEmails := make(map[string]bool)

	// Load existing config if possible
	var loadedCfg config.Config
	if cfg, err := config.LoadConfig("vibegang.yaml"); err == nil {
		loadedCfg = *cfg
	} else {
		loadedCfg.SSHKeyPath = "<none>"
	}

	var companyName, mailDomain string
	var sshKeyPath string
	var userName, userEmail string
	var pmName, pmEmail string
	var maintName, maintEmail string
	var secName, secEmail string
	var model string

	companyName = loadedCfg.CompanyName
	mailDomain = loadedCfg.MailDomain
	if mailDomain == "" {
		mailDomain = "vibegang.local"
	}
	sshKeyPath = loadedCfg.SSHKeyPath
	userName = loadedCfg.UserName
	userEmail = loadedCfg.UserEmail
	model = loadedCfg.Model

	for _, a := range loadedCfg.Agents {
		if a.Role == "pm" {
			pmName, pmEmail = a.Name, a.Email
		} else if a.Role == "maint" {
			maintName, maintEmail = a.Name, a.Email
		} else if a.Role == "sec" {
			secName, secEmail = a.Name, a.Email
		}
	}

	type extraAgent struct {
		nameInput  *tview.InputField
		emailInput *tview.InputField
	}
	var devs []extraAgent
	var tests []extraAgent

	// Forward declaration of updatePreview
	var updatePreview func()

	// Helpers
	addAgentFields := func(titlePrefix string, slice *[]extraAgent, defName, defEmail string) {
		idx := len(*slice) + 1

		nameLabel := fmt.Sprintf("%s %d Name", titlePrefix, idx)
		emailLabel := fmt.Sprintf("%s %d Email", titlePrefix, idx)

		nameInput := tview.NewInputField().SetLabel(nameLabel).SetFieldWidth(30).SetText(defName)
		emailInput := tview.NewInputField().SetLabel(emailLabel).SetFieldWidth(40).SetText(defEmail)

		nameInput.SetChangedFunc(func(text string) {
			if !emailInput.HasFocus() {
				email := generateEmail(text, mailDomain, usedEmails)
				emailInput.SetText(email)
			}
			if updatePreview != nil {
				updatePreview()
			}
		})

		emailInput.SetChangedFunc(func(text string) {
			if updatePreview != nil {
				updatePreview()
			}
		})

		*slice = append(*slice, extraAgent{nameInput, emailInput})
		form.AddFormItem(nameInput)
		form.AddFormItem(emailInput)
	}

	var sshKeys []string
	sshKeys = append(sshKeys, "<none>")

	if home, err := os.UserHomeDir(); err == nil {
		sshDir := filepath.Join(home, ".ssh")
		if entries, err := os.ReadDir(sshDir); err == nil {
			for _, e := range entries {
				name := e.Name()
				if !e.IsDir() && !strings.HasSuffix(name, ".pub") && name != "authorized_keys" && name != "config" && name != "known_hosts" && name != "known_hosts.old" {
					sshKeys = append(sshKeys, filepath.Join(sshDir, name))
				}
			}
		}
	}

	if sshKeyPath == "" {
		sshKeyPath = "<none>"
	}

	initialKeyIndex := 0
	for i, k := range sshKeys {
		if k == sshKeyPath {
			initialKeyIndex = i
			break
		}
	}
	sshKeyPath = sshKeys[initialKeyIndex]

	modelsList := []string{
		"googleai/gemini-3.5-flash",
		"googleai/gemini-3.1-pro-preview",
		"googleai/gemini-3.1-flash-lite",
		"openai/gpt-5.5",
		"openai/gpt-5.5-pro",
		"openai/gpt-5.4",
		"anthropic/claude-sonnet-4-6",
		"anthropic/claude-opus-4-8",
		"togetherai/deepseek-ai/DeepSeek-V4-Pro",
		"togetherai/zai-org/GLM-5.1",
		"togetherai/moonshotai/Kimi-K2.6",
		"togetherai/Qwen/Qwen-3.5-397B-A17B",
		"togetherai/MiniMaxAI/MiniMax-M2.7",
		"togetherai/google/gemma-4-31B-it",
		"custom/kimi-k2.6",
		"custom/kimi-k2.5",
		"custom/minimax-m2.7",
	}
	initialModelIndex := 0
	for i, m := range modelsList {
		if m == model {
			initialModelIndex = i
			break
		}
	}
	if model != "" && initialModelIndex == 0 && modelsList[0] != model {
		modelsList = append(modelsList, model)
		initialModelIndex = len(modelsList) - 1
	}

	companyNameInput := tview.NewInputField().SetLabel("Company Name").SetFieldWidth(30).SetText(companyName)
	mailDomainInput := tview.NewInputField().SetLabel("Mail Domain").SetFieldWidth(30).SetText(mailDomain)

	// User
	userNameInput := tview.NewInputField().SetLabel("Your Name (User)").SetFieldWidth(30).SetText(userName)
	userEmailInput := tview.NewInputField().SetLabel("Your Email (User)").SetFieldWidth(40).SetText(userEmail)

	// PM
	pmNameInput := tview.NewInputField().SetLabel("Project Manager Name").SetFieldWidth(30).SetText(pmName)
	pmEmailInput := tview.NewInputField().SetLabel("Project Manager Email").SetFieldWidth(40).SetText(pmEmail)

	// Maintainer
	maintNameInput := tview.NewInputField().SetLabel("Maintainer Name").SetFieldWidth(30).SetText(maintName)
	maintEmailInput := tview.NewInputField().SetLabel("Maintainer Email").SetFieldWidth(40).SetText(maintEmail)

	// Security
	secNameInput := tview.NewInputField().SetLabel("Security Spec Name").SetFieldWidth(30).SetText(secName)
	secEmailInput := tview.NewInputField().SetLabel("Security Spec Email").SetFieldWidth(40).SetText(secEmail)

	companyNameInput.SetChangedFunc(func(t string) {
		companyName = t
		if updatePreview != nil {
			updatePreview()
		}
	})

	mailDomainInput.SetChangedFunc(func(t string) {
		mailDomain = t
		userEmailInput.SetText(generateEmail(userNameInput.GetText(), mailDomain, usedEmails))
		pmEmailInput.SetText(generateEmail(pmNameInput.GetText(), mailDomain, usedEmails))
		maintEmailInput.SetText(generateEmail(maintNameInput.GetText(), mailDomain, usedEmails))
		secEmailInput.SetText(generateEmail(secNameInput.GetText(), mailDomain, usedEmails))
		for _, d := range devs {
			d.emailInput.SetText(generateEmail(d.nameInput.GetText(), mailDomain, usedEmails))
		}
		for _, t := range tests {
			t.emailInput.SetText(generateEmail(t.nameInput.GetText(), mailDomain, usedEmails))
		}
		if updatePreview != nil {
			updatePreview()
		}
	})

	userNameInput.SetChangedFunc(func(t string) {
		userEmailInput.SetText(generateEmail(t, mailDomain, usedEmails))
		if updatePreview != nil {
			updatePreview()
		}
	})
	userEmailInput.SetChangedFunc(func(t string) {
		if updatePreview != nil {
			updatePreview()
		}
	})

	pmNameInput.SetChangedFunc(func(t string) {
		pmEmailInput.SetText(generateEmail(t, mailDomain, usedEmails))
		if updatePreview != nil {
			updatePreview()
		}
	})
	pmEmailInput.SetChangedFunc(func(t string) {
		if updatePreview != nil {
			updatePreview()
		}
	})

	maintNameInput.SetChangedFunc(func(t string) {
		maintEmailInput.SetText(generateEmail(t, mailDomain, usedEmails))
		if updatePreview != nil {
			updatePreview()
		}
	})
	maintEmailInput.SetChangedFunc(func(t string) {
		if updatePreview != nil {
			updatePreview()
		}
	})

	secNameInput.SetChangedFunc(func(t string) {
		secEmailInput.SetText(generateEmail(t, mailDomain, usedEmails))
		if updatePreview != nil {
			updatePreview()
		}
	})
	secEmailInput.SetChangedFunc(func(t string) {
		if updatePreview != nil {
			updatePreview()
		}
	})

	form.AddFormItem(companyNameInput)
	form.AddFormItem(mailDomainInput)

	form.AddDropDown("SSH Key Path", sshKeys, initialKeyIndex, func(option string, optionIndex int) {
		sshKeyPath = option
		if updatePreview != nil {
			updatePreview()
		}
	})

	form.AddDropDown("AI Model", modelsList, initialModelIndex, func(option string, optionIndex int) {
		model = option
		if updatePreview != nil {
			updatePreview()
		}
	})

	form.AddFormItem(userNameInput)
	form.AddFormItem(userEmailInput)
	form.AddFormItem(pmNameInput)
	form.AddFormItem(pmEmailInput)
	form.AddFormItem(maintNameInput)
	form.AddFormItem(maintEmailInput)
	form.AddFormItem(secNameInput)
	form.AddFormItem(secEmailInput)

	// Existing extra agents
	devCount, testCount := 0, 0
	for _, a := range loadedCfg.Agents {
		if a.Role == "dev" {
			addAgentFields("Software Engineer", &devs, a.Name, a.Email)
			devCount++
		} else if a.Role == "test" {
			addAgentFields("Test Engineer", &tests, a.Name, a.Email)
			testCount++
		}
	}

	if devCount == 0 {
		addAgentFields("Software Engineer", &devs, "", "")
	}
	if testCount == 0 {
		addAgentFields("Test Engineer", &tests, "", "")
	}

	form.AddButton("Add Developer", func() {
		addAgentFields("Software Engineer", &devs, "", "")
		if updatePreview != nil {
			updatePreview()
		}
	})

	form.AddButton("Add Tester", func() {
		addAgentFields("Test Engineer", &tests, "", "")
		if updatePreview != nil {
			updatePreview()
		}
	})

	saved := false

	form.AddButton("Save & Exit", func() {
		companyName = companyNameInput.GetText()
		mailDomain = mailDomainInput.GetText()
		pmName = pmNameInput.GetText()
		pmEmail = pmEmailInput.GetText()
		maintName = maintNameInput.GetText()
		maintEmail = maintEmailInput.GetText()
		secName = secNameInput.GetText()
		secEmail = secEmailInput.GetText()
		userName = userNameInput.GetText()
		userEmail = userEmailInput.GetText()

		var devNames, devEmails []string
		for _, d := range devs {
			n := d.nameInput.GetText()
			e := d.emailInput.GetText()
			if n != "" {
				devNames = append(devNames, n)
				devEmails = append(devEmails, e)
			}
		}

		var testNames, testEmails []string
		for _, t := range tests {
			n := t.nameInput.GetText()
			e := t.emailInput.GetText()
			if n != "" {
				testNames = append(testNames, n)
				testEmails = append(testEmails, e)
			}
		}

		getExistingPrompt := func(role, email, name string, defaultPrompt string) string {
			for _, a := range loadedCfg.Agents {
				if a.Email == email && a.Role == role {
					if a.SystemPrompt != "" {
						return a.SystemPrompt
					}
				}
			}
			for _, a := range loadedCfg.Agents {
				if a.Role == role && (role == "pm" || role == "maint" || role == "sec") {
					if a.SystemPrompt != "" {
						return a.SystemPrompt
					}
				}
			}
			for _, a := range loadedCfg.Agents {
				if a.Name == name && a.Role == role {
					if a.SystemPrompt != "" {
						return a.SystemPrompt
					}
				}
			}
			return defaultPrompt
		}

		cfg := config.Config{
			CompanyName: companyName,
			MailDomain:  mailDomain,
			SSHKeyPath:  sshKeyPath,
			UserName:    userName,
			UserEmail:   userEmail,
			Model:       model,
			Agents: []config.AgentConfig{
				{
					Name:         pmName,
					Email:        pmEmail,
					Role:         "pm",
					SystemPrompt: getExistingPrompt("pm", pmEmail, pmName, config.PMSystemPrompt),
					Tools:        []string{"check_mailbox", "read_mail", "send_mail"},
				},
				{
					Name:         maintName,
					Email:        maintEmail,
					Role:         "maint",
					SystemPrompt: getExistingPrompt("maint", maintEmail, maintName, config.MaintSystemPrompt),
					Tools:        []string{"check_mailbox", "read_mail", "send_mail", "run_terminal_command"},
				},
				{
					Name:         secName,
					Email:        secEmail,
					Role:         "sec",
					SystemPrompt: getExistingPrompt("sec", secEmail, secName, config.SecSystemPrompt),
					Tools:        []string{"check_mailbox", "read_mail", "send_mail", "read_file", "run_terminal_command"},
				},
			},
		}

		for _, d := range devs {
			n := d.nameInput.GetText()
			e := d.emailInput.GetText()
			if n != "" {
				cfg.Agents = append(cfg.Agents, config.AgentConfig{
					Name:         n,
					Email:        e,
					Role:         "dev",
					SystemPrompt: getExistingPrompt("dev", e, n, config.DevSystemPrompt),
					Tools:        []string{"check_mailbox", "read_mail", "send_mail", "read_file", "write_file", "run_terminal_command"},
				})
			}
		}

		for _, t := range tests {
			n := t.nameInput.GetText()
			e := t.emailInput.GetText()
			if n != "" {
				cfg.Agents = append(cfg.Agents, config.AgentConfig{
					Name:         n,
					Email:        e,
					Role:         "test",
					SystemPrompt: getExistingPrompt("test", e, n, config.TestSystemPrompt),
					Tools:        []string{"check_mailbox", "read_mail", "send_mail", "read_file", "write_file", "run_terminal_command"},
				})
			}
		}

		data, err := yaml.Marshal(&cfg)
		if err == nil {
			os.WriteFile("vibegang.yaml", data, 0644)
			saved = true
		}
		app.Stop()
	})

	form.AddButton("Quit", func() {
		app.Stop()
	})

	// Setup updatePreview logic
	updatePreview = func() {
		var sb strings.Builder
		sb.WriteString("\n")
		sb.WriteString(" [#bb9af7]⚡ VIBEGANG ROSTER PREVIEW ⚡[-]\n")
		sb.WriteString(" ───────────────────────────────\n\n")

		cName := companyNameInput.GetText()
		if cName == "" {
			cName = "[#565f89]Not Set[-]"
		}
		mDom := mailDomainInput.GetText()
		if mDom == "" {
			mDom = "[#565f89]Not Set[-]"
		}

		sb.WriteString("  [#e0af68]■ ENVIRONMENT[-]\n")
		sb.WriteString(fmt.Sprintf("    [#565f89]Company:[-] %s\n", cName))
		sb.WriteString(fmt.Sprintf("    [#565f89]Domain:[-]  %s\n", mDom))
		sb.WriteString(fmt.Sprintf("    [#565f89]Key:[-]     %s\n", sshKeyPath))
		sb.WriteString(fmt.Sprintf("    [#565f89]Model:[-]   %s\n\n", model))

		sb.WriteString("  [#e0af68]■ CORE TEAM[-]\n")
		uName := userNameInput.GetText()
		if uName == "" {
			uName = "[#565f89]Not Set[-]"
		}
		sb.WriteString(fmt.Sprintf("    [#7aa2f7]End User:[-]  %s (%s)\n", uName, userEmailInput.GetText()))

		pName := pmNameInput.GetText()
		if pName == "" {
			pName = "[#565f89]Not Set[-]"
		}
		sb.WriteString(fmt.Sprintf("    [#7aa2f7]Manager:[-]   %s (%s)\n", pName, pmEmailInput.GetText()))

		mName := maintNameInput.GetText()
		if mName == "" {
			mName = "[#565f89]Not Set[-]"
		}
		sb.WriteString(fmt.Sprintf("    [#7aa2f7]Maintain:[-]  %s (%s)\n", mName, maintEmailInput.GetText()))

		sName := secNameInput.GetText()
		if sName == "" {
			sName = "[#565f89]Not Set[-]"
		}
		sb.WriteString(fmt.Sprintf("    [#7aa2f7]Security:[-]  %s (%s)\n\n", sName, secEmailInput.GetText()))

		sb.WriteString("  [#e0af68]■ SOFTWARE ENGINEERS[-]\n")
		hasDevs := false
		for _, d := range devs {
			n := d.nameInput.GetText()
			if n != "" {
				sb.WriteString(fmt.Sprintf("    [#9ece6a]•[-] %s (%s)\n", n, d.emailInput.GetText()))
				hasDevs = true
			}
		}
		if !hasDevs {
			sb.WriteString("    [#565f89](No developers added yet)[-]\n")
		}
		sb.WriteString("\n")

		sb.WriteString("  [#e0af68]■ TEST ENGINEERS[-]\n")
		hasTests := false
		for _, t := range tests {
			n := t.nameInput.GetText()
			if n != "" {
				sb.WriteString(fmt.Sprintf("    [#9ece6a]•[-] %s (%s)\n", n, t.emailInput.GetText()))
				hasTests = true
			}
		}
		if !hasTests {
			sb.WriteString("    [#565f89](No test engineers added yet)[-]\n")
		}

		sb.WriteString("\n\n")
		sb.WriteString(" ───────────────────────────────\n")
		sb.WriteString("  [#565f89]Navigation Hints:\n")
		sb.WriteString("   • Tab / Shift-Tab: Move focus\n")
		sb.WriteString("   • Esc: Quit setup and discard\n")
		sb.WriteString("   • Enter: Activate buttons[-]\n")

		previewView.SetText(sb.String())
	}

	// Trigger initial preview build
	updatePreview()

	// Title / Header
	headerView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	headerView.SetText(`
[#bb9af7]V I B E G A N G   M U L T I - A G E N T   H A R N E S S[-]
[#565f89]Establish and modify your dynamic autonomous software development team[-]
`)

	// Layout everything in a split screen grid
	mainFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(form, 0, 3, true).
		AddItem(previewView, 0, 2, false)

	rootFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(headerView, 3, 1, false).
		AddItem(mainFlex, 0, 1, true)

	pages := tview.NewPages()
	pages.AddPage("main", rootFlex, true, true)

	modal := tview.NewModal().
		SetText("Are you sure you want to exit and discard all changes?").
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Yes" {
				app.Stop()
			} else {
				pages.HidePage("modal")
			}
		})

	pages.AddPage("modal", modal, false, false)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			if name, _ := pages.GetFrontPage(); name == "modal" {
				pages.HidePage("modal")
				return nil
			}
			pages.ShowPage("modal")
			return nil
		}
		return event
	})

	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

	if saved {
		fmt.Println("Setup completed. Configuration saved to vibegang.yaml.")
	} else {
		fmt.Println("Setup cancelled. No changes were saved.")
	}
}
