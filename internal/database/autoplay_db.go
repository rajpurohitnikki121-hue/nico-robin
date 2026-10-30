package database

// AutoPlayEnabled returns whether autoplay is enabled for the chat.
// Autoplay is off by default.
func AutoPlayEnabled(chatID int64) (bool, error) {
	settings, err := getChatSettings(chatID)
	if err != nil {
		return false, err
	}
	return settings.AutoPlay, nil
}

// SetAutoPlay turns autoplay on or off for the chat.
func SetAutoPlay(chatID int64, enabled bool) error {
	return modifyChatSettings(chatID, func(s *ChatSettings) bool {
		if s.AutoPlay == enabled {
			return false
		}
		s.AutoPlay = enabled
		return true
	})
}
