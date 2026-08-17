package bot

// InlineKeyboardMarkup / InlineKeyboardButton are minimal types so this
// package does not depend on a Telegram library. The HTTP API accepts the
// JSON shape we produce here verbatim.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// ReplyKeyboardMarkup / KeyboardButton for the contact-request prompt.
type KeyboardButton struct {
	Text           string `json:"text"`
	RequestContact bool   `json:"request_contact,omitempty"`
}

type ReplyKeyboardMarkup struct {
	Keyboard        [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool               `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard bool               `json:"one_time_keyboard,omitempty"`
}

// removeReplyKeyboard is the canonical "hide custom keyboard" payload.
func removeReplyKeyboard() map[string]interface{} {
	return map[string]interface{}{"remove_keyboard": true}
}

// 2-column row helper.
func row(buttons ...InlineKeyboardButton) []InlineKeyboardButton {
	return buttons
}

// ServiceOptions: 3 choices for what the user is looking for.
func ServiceOptions() InlineKeyboardMarkup {
	return InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{
		row(
			InlineKeyboardButton{Text: "Buy Property", CallbackData: "service:buy"},
			InlineKeyboardButton{Text: "Rent Property", CallbackData: "service:rent"},
		),
		row(
			InlineKeyboardButton{Text: "Sell Property", CallbackData: "service:sell"},
		),
	}}
}

// PropertyTypeOptions for the real-estate MVP.
func PropertyTypeOptions() InlineKeyboardMarkup {
	return InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{
		row(
			InlineKeyboardButton{Text: "Apartment", CallbackData: "prop:apartment"},
			InlineKeyboardButton{Text: "House", CallbackData: "prop:house"},
		),
		row(
			InlineKeyboardButton{Text: "Villa", CallbackData: "prop:villa"},
			InlineKeyboardButton{Text: "Commercial", CallbackData: "prop:commercial"},
		),
	}}
}

// TimelineOptions matches the spec.
func TimelineOptions() InlineKeyboardMarkup {
	return InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{
		row(InlineKeyboardButton{Text: "Immediately", CallbackData: "timeline:immediately"}),
		row(InlineKeyboardButton{Text: "1-3 months", CallbackData: "timeline:1-3 months"}),
		row(InlineKeyboardButton{Text: "3-6 months", CallbackData: "timeline:3-6 months"}),
		row(InlineKeyboardButton{Text: "Just researching", CallbackData: "timeline:researching"}),
	}}
}

// ConfirmOptions for the final confirmation step.
func ConfirmOptions() InlineKeyboardMarkup {
	return InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{
		row(
			InlineKeyboardButton{Text: "Confirm", CallbackData: "confirm:yes"},
			InlineKeyboardButton{Text: "Edit", CallbackData: "confirm:edit"},
			InlineKeyboardButton{Text: "Cancel", CallbackData: "confirm:cancel"},
		),
	}}
}

// ContactKeyboard asks Telegram to share the phone number.
func ContactKeyboard() ReplyKeyboardMarkup {
	return ReplyKeyboardMarkup{
		Keyboard: [][]KeyboardButton{
			{{Text: "Share phone number", RequestContact: true}},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: true,
	}
}

// Service label lookup; callback data is short, the user sees the long form.
func serviceLabel(code string) string {
	switch code {
	case "buy":
		return "Buy Property"
	case "rent":
		return "Rent Property"
	case "sell":
		return "Sell Property"
	}
	return code
}

func propertyLabel(code string) string {
	switch code {
	case "apartment":
		return "Apartment"
	case "house":
		return "House"
	case "villa":
		return "Villa"
	case "commercial":
		return "Commercial"
	}
	return code
}

func timelineLabel(code string) string {
	switch code {
	case "immediately":
		return "Immediately"
	case "1-3 months":
		return "1-3 months"
	case "3-6 months":
		return "3-6 months"
	case "researching":
		return "Just researching"
	}
	return code
}
