package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/gosthome/icons/fynico"
	_ "github.com/gosthome/icons/fynico/google/materialdesigniconsoutlined"
)

// The current gosthome icon pack exposes Material Design styles (filled/outlined/round/sharp),
// but not Material Symbols weight variants (for example, weight 200).
const googleMaterialCollection = "materialdesigniconsoutlined"

func materialIcon(name string, fallback fyne.Resource) fyne.Resource {
	if res := fynico.Collections.Lookup(googleMaterialCollection, name); res != nil {
		return theme.NewThemedResource(res)
	}
	return fallback
}

func materialPrimaryIcon(name string, fallback fyne.Resource) fyne.Resource {
	if res := fynico.Collections.Lookup(googleMaterialCollection, name); res != nil {
		return theme.NewPrimaryThemedResource(res)
	}
	return theme.NewPrimaryThemedResource(fallback)
}

func iconContentCopy() fyne.Resource { return materialIcon("content_copy", theme.ContentCopyIcon()) }
func iconRefresh() fyne.Resource     { return materialIcon("refresh", theme.ViewRefreshIcon()) }
func iconHistory() fyne.Resource     { return materialIcon("history", theme.HistoryIcon()) }
func iconSearch() fyne.Resource      { return materialIcon("search", theme.SearchIcon()) }
func iconSettings() fyne.Resource    { return materialIcon("settings", theme.SettingsIcon()) }
func iconDocument() fyne.Resource    { return materialIcon("description", theme.DocumentIcon()) }
func iconCancel() fyne.Resource      { return materialIcon("cancel", theme.CancelIcon()) }
func iconAdd() fyne.Resource         { return materialIcon("add", theme.ContentAddIcon()) }
func iconWifi() fyne.Resource        { return materialIcon("wifi", theme.RadioButtonCheckedIcon()) }
func iconWifiOff() fyne.Resource     { return materialIcon("wifi_off", theme.RadioButtonIcon()) }
func iconAttachFile() fyne.Resource  { return materialIcon("attach_file", theme.MailAttachmentIcon()) }
func iconFolderOpen() fyne.Resource  { return materialIcon("folder_open", theme.FolderOpenIcon()) }
func iconSendPrimary() fyne.Resource { return materialPrimaryIcon("send", theme.MailSendIcon()) }
