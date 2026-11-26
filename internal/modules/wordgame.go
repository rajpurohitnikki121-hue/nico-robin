package modules

import (
	"fmt"
	"html"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Laky-64/gologging"
	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/core"
	"main/internal/database"
	"main/internal/utils"
)

// Word Scramble game: one round = guess the MOVIE, then the SONG of that
// movie, one after the other. Each part has its own 30 second timer and the
// same 3-hint system:
//   Hint 1 - shown to everybody automatically
//   Hint 2 - button, reveals the first letter
//   Hint 3 - button, reveals the first, middle and last letters

const wordGameTime = 30 * time.Second

type wgEntry struct {
	Movie string
	Song  string
	Clue  string // hint 1 of the movie round (about the movie, not the song)
}

var wgData = []wgEntry{
	{"Aashiqui 2", "Tum Hi Ho", "A broken singer and the girl he makes a star (2013)"},
	{"Kabir Singh", "Bekhayali", "Shahid Kapoor as an angry surgeon in love (2019)"},
	{"Yeh Jawaani Hai Deewani", "Balam Pichkari", "Ranbir and Deepika, a Manali trek with friends (2013)"},
	{"Dilwale Dulhania Le Jayenge", "Tujhe Dekha To Jaana Sanam", "Raj and Simran, Europe trip and mustard fields (1995)"},
	{"Kabhi Khushi Kabhie Gham", "Suraj Hua Maddham", "Karan Johar's big family drama, Yash and Nandini's home"},
	{"Kuch Kuch Hota Hai", "Tujhe Yaad Na Meri Aayi", "Rahul, Anjali and Tina, a summer camp story (1998)"},
	{"Rockstar", "Kun Faya Kun", "Ranbir Kapoor as a heartbroken singer who loves Heer (2011)"},
	{"Barfi", "Phir Le Aya Dil", "Ranbir Kapoor as a hero who cannot hear or speak, Darjeeling (2012)"},
	{"3 Idiots", "All Izz Well", "Aamir Khan as Rancho in an engineering college (2009)"},
	{"Chennai Express", "Kashmir Main Tu Kanyakumari", "Shah Rukh Khan and Deepika on a train journey (2013)"},
	{"Jab Tak Hai Jaan", "Challa", "Shah Rukh Khan's London love story (2012)"},
	{"Ae Dil Hai Mushkil", "Channa Mereya", "Ranbir, Anushka and Aishwarya, one-sided love (2016)"},
	{"Raazi", "Ae Watan", "Alia Bhatt as a young spy (2018)"},
	{"Sanju", "Main Badhiya Tu Bhi Badhiya", "Sanjay Dutt's biopic with Ranbir Kapoor (2018)"},
	{"Gully Boy", "Apna Time Aayega", "Ranveer Singh as a rapper from a Mumbai slum (2019)"},
	{"Dangal", "Haanikaarak Bapu", "Aamir Khan, a wrestler father and his daughters (2016)"},
	{"Bajrangi Bhaijaan", "Selfie Le Le Re", "Salman Khan helps a lost girl reach Pakistan (2015)"},
	{"Tamasha", "Agar Tum Saath Ho", "Ranbir and Deepika, a story that starts in Corsica (2015)"},
	{"Student Of The Year", "Disco Deewane", "Alia, Varun and Sidharth's debut, college drama (2012)"},
	{"Dil Chahta Hai", "Koi Kahe Kehta Rahe", "Three friends and a Goa trip (2001)"},
	{"Zindagi Na Milegi Dobara", "Senorita", "Three friends on a Spain road trip (2011)"},
	{"Veer Zaara", "Tere Liye", "An Indian pilot and a Pakistani girl (2004)"},
	{"Devdas", "Dola Re Dola", "Shah Rukh, Aishwarya and Madhuri in a grand-set film (2002)"},
	{"Bajirao Mastani", "Deewani Mastani", "Ranveer, Deepika and Priyanka, a Peshwa's story (2015)"},
	{"Padmaavat", "Ghoomar", "Deepika, Ranveer and Shahid, the queen of Chittorgarh (2018)"},
	{"Animal", "Arjan Vailly", "Ranbir Kapoor in an intense father-son film (2023)"},
	{"Jawan", "Chaleya", "Shah Rukh Khan in Atlee's action film (2023)"},
	{"Pathaan", "Besharam Rang", "Shah Rukh Khan as a spy (2023)"},
	{"Shershaah", "Raataan Lambiyan", "Sidharth Malhotra as Kargil hero Captain Vikram Batra (2021)"},
	{"Brahmastra", "Kesariya", "Ranbir and Alia, the story of Shiva and the Astra (2022)"},
	{"Wake Up Sid", "Iktara", "Ranbir Kapoor as a carefree Mumbai boy (2009)"},
	{"Lagaan", "O Rey Chhori", "Villagers play a cricket match against the British (2001)"},
	{"Swades", "Yeh Tara Woh Tara", "Shah Rukh Khan, a NASA engineer who returns to a village (2004)"},
	{"Om Shanti Om", "Dard E Disco", "Shah Rukh Khan in a rebirth story (2007)"},
	{"Jab We Met", "Yeh Ishq Haaye", "Kareena and Shahid, a talkative girl on a train (2007)"},
	{"Rab Ne Bana Di Jodi", "Haule Haule", "Shah Rukh Khan and Anushka, Surinder and Raj (2008)"},
	{"Highway", "Patakha Guddi", "Alia Bhatt and Randeep Hooda on a road trip (2014)"},
	{"Stree", "Aaj Ki Raat", "Shraddha and Rajkummar, the fear of Chanderi (2018)"},
	{"Gadar", "Udja Kale Kawan", "Sunny Deol as Tara Singh, love across the border (2001)"},
	{"Border", "Sandese Aate Hain", "A patriotic war film based on 1971 (1997)"},
	{"Sholay", "Yeh Dosti", "Jai, Veeru and Gabbar of Ramgarh (1975)"},
	{"Mughal E Azam", "Pyar Kiya To Darna Kya", "The eternal love of Salim and Anarkali (1960)"},
	{"Mr India", "Kate Nahin Katte", "Anil Kapoor and Sridevi, Mogambo is happy (1987)"},
	{"Tezaab", "Ek Do Teen", "Anil Kapoor and Madhuri Dixit's superhit (1988)"},
	{"Kaho Naa Pyaar Hai", "Ek Pal Ka Jeena", "Hrithik Roshan's debut film (2000)"},
	{"Aashiqui", "Dheere Dheere", "Rahul Roy's 90s musical hit (1990)"},
	{"Cocktail", "Tum Hi Ho Bandhu", "Saif, Deepika and Diana, a London love triangle (2012)"},
	{"Kal Ho Naa Ho", "Maahi Ve", "Shah Rukh, Preity and Saif in New York (2003)"},
	{"Kabhi Alvida Naa Kehna", "Mitwa", "A story of marriages and second chances, Shah Rukh and Rani (2006)"},
	{"Ghajini", "Guzarish", "Aamir Khan as a man with short-term memory loss (2008)"},
	{"Sultan", "Baby Ko Bass Pasand Hai", "Salman Khan as a wrestler from Haryana (2016)"},
	{"Tiger Zinda Hai", "Swag Se Swagat", "Salman and Katrina rescue nurses in Iraq (2017)"},
	{"Ek Tha Tiger", "Mashallah", "Salman Khan and Katrina Kaif as spies (2012)"},
	{"Dabangg", "Munni Badnaam Hui", "Salman Khan as cop Chulbul Pandey (2010)"},
	{"Kick", "Jumme Ki Raat", "Salman Khan as a thrill-seeking hero (2014)"},
	{"Happy New Year", "Manwa Laage", "Shah Rukh Khan's dance competition heist in Dubai (2014)"},
	{"Dilwale", "Gerua", "Shah Rukh Khan and Kajol reunite on screen (2015)"},
	{"Raees", "Zaalima", "Shah Rukh Khan as a liquor baron of Gujarat (2017)"},
	{"Don", "Main Hoon Don", "Shah Rukh Khan as the underworld boss (2006)"},
	{"Bang Bang", "Tu Meri", "Hrithik Roshan and Katrina Kaif in an action thriller (2014)"},
	{"War", "Ghungroo", "Hrithik Roshan vs Tiger Shroff (2019)"},
	{"Kapoor And Sons", "Kar Gayi Chull", "Sidharth, Alia and Fawad, a family drama (2016)"},
	{"Badrinath Ki Dulhania", "Tamma Tamma Again", "Varun Dhawan and Alia Bhatt, Jhansi to Singapore (2017)"},
	{"Humpty Sharma Ki Dulhania", "Samjhawan", "Varun and Alia, Ambala to Kota (2014)"},
	{"2 States", "Offo", "Arjun Kapoor and Alia, a Punjabi-Tamil love story (2014)"},
	{"Chhichhore", "Khairiyat", "Sushant Singh Rajput, a college friends reunion (2019)"},
	{"Kai Po Che", "Manjha", "Three friends, cricket and Gujarat (2013)"},
	{"MS Dhoni The Untold Story", "Kaun Tujhe", "Sushant Singh Rajput as the cricket captain (2016)"},
	{"Bhaag Milkha Bhaag", "Zinda", "Farhan Akhtar as the Flying Sikh (2013)"},
	{"Mary Kom", "Sukoon Mila", "Priyanka Chopra as a boxing champion (2014)"},
	{"Taare Zameen Par", "Maa", "Aamir Khan and a dyslexic boy (2007)"},
	{"Rang De Basanti", "Luka Chuppi", "Aamir Khan and college friends who turn revolutionaries (2006)"},
	{"PK", "Nanga Punga Dost", "Aamir Khan as an alien on Earth (2014)"},
	{"Jodhaa Akbar", "Azeem O Shaan Shahenshah", "Hrithik and Aishwarya, a Mughal emperor's love (2008)"},
	{"Taal", "Ramta Jogi", "Aishwarya Rai, Akshaye Khanna and the music industry (1999)"},
	{"Dil To Pagal Hai", "Le Gayi", "Shah Rukh, Madhuri and Karisma in a dance musical (1997)"},
	{"Hum Aapke Hain Koun", "Didi Tera Devar Deewana", "Salman and Madhuri in a big family wedding film (1994)"},
	{"Maine Pyar Kiya", "Dil Deewana", "Salman Khan's debut as lead (1989)"},
	{"Hum Dil De Chuke Sanam", "Nimbooda", "Salman, Aishwarya and Ajay Devgn love triangle (1999)"},
	{"Dil Se", "Chaiyya Chaiyya", "Shah Rukh Khan and Manisha Koirala, a song on top of a train (1998)"},
	{"Anand", "Kahin Door Jab Din Dhal Jaye", "Rajesh Khanna as a cheerful man with a terminal illness (1971)"},
	{"Pakeezah", "Chalte Chalte", "Meena Kumari as a courtesan (1972)"},
	{"Amar Akbar Anthony", "My Name Is Anthony Gonsalves", "Three brothers separated in childhood (1977)"},
	{"Silsila", "Rang Barse", "Amitabh, Rekha and Jaya, a love triangle (1981)"},
	{"Disco Dancer", "Jimmy Jimmy Aaja", "Mithun Chakraborty as a street singer-dancer (1982)"},
	{"Qayamat Se Qayamat Tak", "Papa Kehte Hain", "Aamir Khan's debut, a Romeo-Juliet style story (1988)"},
	{"Dil", "Mujhe Neend Na Aaye", "Aamir Khan and Madhuri Dixit (1990)"},
	{"Baazigar", "Yeh Kaali Kaali Aankhen", "Shah Rukh Khan as a revenge-seeking anti-hero (1993)"},
	{"Darr", "Jaadu Teri Nazar", "Shah Rukh Khan as an obsessive lover (1993)"},
	{"Khal Nayak", "Choli Ke Peeche", "Sanjay Dutt and Madhuri Dixit (1993)"},
	{"Hum Saath Saath Hain", "Mhare Hiwda Mein Nache Mor", "A joint-family film with Salman, Saif and Karisma (1999)"},
	{"Murder", "Bheegey Hont Tere", "Emraan Hashmi and Mallika Sherawat (2004)"},
	{"Gangster", "Tu Hi Meri Shab Hai", "Emraan Hashmi and Kangana Ranaut (2006)"},
	{"Jannat", "Zara Sa", "Emraan Hashmi as a cricket-betting hustler (2008)"},
	{"The Dirty Picture", "Ooh La La", "Vidya Balan as a bold south film star (2011)"},
	{"Tanu Weds Manu", "Sadi Gali", "Kangana Ranaut and R Madhavan, a Kanpur wedding drama (2011)"},
	{"Queen", "London Thumakda", "Kangana Ranaut goes alone on her honeymoon trip (2014)"},
	{"Piku", "Journey Song", "Deepika, Amitabh and Irrfan, a road trip from Delhi to Kolkata (2015)"},
	{"Ram Leela", "Nagada Sang Dhol", "Ranveer and Deepika, a Romeo-Juliet story in Gujarat (2013)"},
	{"Simmba", "Aankh Marey", "Ranveer Singh as a corrupt cop turned honest (2018)"},
}

type wordGame struct {
	mu      sync.Mutex
	chatID  int64
	starter int64
	entry   wgEntry
	stage   int // 1 = movie, 2 = song
	gen     int // changes on every round, stale timers are ignored
	h2, h3  bool
	started time.Time
	msg     *tg.NewMessage
	timer   *time.Timer
	scr     string
}

var (
	wgMu    sync.Mutex
	wgGames = map[int64]*wordGame{}
	// wgUsed remembers which entries each chat already had, so nothing
	// repeats until all of them were played once.
	wgUsed = map[int64]map[int]bool{}
)

// wgPick returns an entry this chat has not seen yet. Caller holds wgMu.
func wgPick(chatID int64) wgEntry {
	used := wgUsed[chatID]
	if used == nil || len(used) >= len(wgData) {
		used = map[int]bool{}
		wgUsed[chatID] = used
	}
	var free []int
	for i := range wgData {
		if !used[i] {
			free = append(free, i)
		}
	}
	i := free[rand.Intn(len(free))]
	used[i] = true
	return wgData[i]
}

func init() {
	helpTexts["/wordgame"] = `<i>Word Scramble: guess the movie first, then a song from that movie.</i>

<u>Usage:</u>
<b>/wordgame</b> — Start a new game (works in groups and in DM)\n<b>/endgame</b> — End the running game (starter or admin)\n<b>/leaderboard</b> — Top 8 players of this chat

<b>💡 Rules:</b>
• Unscramble the movie name first, then the song
• Hint 1 is shown to everyone automatically
• Hint 2 button: reveals the first letter
• Hint 3 button: reveals the first, middle and last letters
• Each part has its own 30 second timer
• Just type your answer in the chat`
}

func wgNorm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// wgScramble shuffles the letters inside every word and makes sure the result
// is different from the original.
func wgScramble(name string) string {
	words := strings.Fields(strings.ToUpper(name))
	for try := 0; try < 20; try++ {
		out := make([]string, len(words))
		for i, w := range words {
			r := []rune(w)
			if len(r) > 2 {
				rand.Shuffle(len(r), func(a, b int) { r[a], r[b] = r[b], r[a] })
			}
			out[i] = strings.Join(strings.Split(string(r), ""), " ")
		}
		res := strings.Join(out, "   ")
		if len(wgNorm(name)) < 3 || wgNorm(res) != wgNorm(name) {
			return res
		}
	}
	return strings.ToUpper(name)
}

// wgMask shows the name with only the chosen letters visible.
// level 2: first letter; level 3: first, middle and last letters.
func wgMask(name string, level int) string {
	total := 0
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			total++
		}
	}
	show := map[int]bool{0: true}
	if level >= 3 {
		show[total-1] = true
		show[total/2] = true
	}
	idx := 0
	var words []string
	for _, w := range strings.Fields(name) {
		var letters []string
		for _, r := range w {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				if show[idx] {
					letters = append(letters, strings.ToUpper(string(r)))
				} else {
					letters = append(letters, "_")
				}
				idx++
			} else {
				letters = append(letters, string(r))
			}
		}
		words = append(words, strings.Join(letters, " "))
	}
	return strings.Join(words, "   ")
}

func (g *wordGame) answer() string {
	if g.stage == 1 {
		return g.entry.Movie
	}
	return g.entry.Song
}

func (g *wordGame) text() string {
	var sb strings.Builder
	if g.stage == 1 {
		sb.WriteString("🎬 <b>Guess The Movie</b>\n\n")
	} else {
		sb.WriteString("🎵 <b>Guess The Song</b>\n\n")
	}
	what := "movie"
	if g.stage == 2 {
		what = "song"
	}
	sb.WriteString("❓ <b>Unscramble the " + what + " name:</b>\n")
	sb.WriteString("👉 <code>" + html.EscapeString(g.scr) + "</code>\n")
	sb.WriteString(fmt.Sprintf("🔤 <i>%d letters</i>\n\n", len([]rune(wgNorm(g.answer())))))
	if g.stage == 1 {
		sb.WriteString("➤ <b>Hint 1:</b> " + html.EscapeString(g.entry.Clue) + "\n")
	} else {
		sb.WriteString("➤ <b>Hint 1:</b> This song is from the movie <b>" + html.EscapeString(g.entry.Movie) + "</b>\n")
	}
	if g.h2 {
		sb.WriteString("➤ <b>Hint 2:</b> <code>" + html.EscapeString(wgMask(g.answer(), 2)) + "</code>\n")
	}
	if g.h3 {
		sb.WriteString("➤ <b>Hint 3:</b> <code>" + html.EscapeString(wgMask(g.answer(), 3)) + "</code>\n")
	}
	sb.WriteString("\n⏱ <i>You have 30 seconds. Type your answer in the chat!</i>")
	return sb.String()
}

func (g *wordGame) markup() *tg.ReplyInlineMarkup {
	// One Hint button: the first tap shows Hint 2, the second shows Hint 3.
	if g.h3 {
		return nil
	}
	kb := tg.NewKeyboard()
	kb.AddRow(tg.Button.Data("💡 Hint", "wg:hint"))
	return kb.Build()
}

// wgSendRound posts the puzzle of the current stage and starts its timer.
func wgSendRound(g *wordGame) {
	g.mu.Lock()
	name := g.answer()
	g.scr = wgScramble(name)
	g.h2, g.h3 = false, false
	g.started = time.Now()
	g.gen++
	gen := g.gen
	text := g.text()
	mk := g.markup()
	g.mu.Unlock()

	opts := &tg.SendOptions{}
	if mk != nil {
		opts.ReplyMarkup = mk
	}
	msg, err := core.Bot.SendMessage(g.chatID, text, opts)
	if err != nil || msg == nil {
		gologging.ErrorF("wordgame: send failed in %d: %v", g.chatID, err)
		wgEnd(g)
		return
	}

	g.mu.Lock()
	if g.gen != gen {
		g.mu.Unlock()
		return
	}
	g.msg = msg
	g.timer = time.AfterFunc(wordGameTime, func() { wgTimeout(g, gen) })
	g.mu.Unlock()
}

func wgEnd(g *wordGame) {
	g.mu.Lock()
	if g.timer != nil {
		g.timer.Stop()
	}
	g.mu.Unlock()
	wgMu.Lock()
	if wgGames[g.chatID] == g {
		delete(wgGames, g.chatID)
	}
	wgMu.Unlock()
}

func wgTimeout(g *wordGame, gen int) {
	g.mu.Lock()
	if g.gen != gen {
		g.mu.Unlock()
		return
	}
	msg := g.msg
	text := "⏰ <b>Time is up!</b>\n\n🎬 Movie: <b>" + html.EscapeString(g.entry.Movie) +
		"</b>\n🎵 Song: <b>" + html.EscapeString(g.entry.Song) +
		"</b>\n\nPlay again: /wordgame"
	g.gen++
	g.mu.Unlock()

	if msg != nil {
		if _, err := msg.Edit(text, &tg.SendOptions{}); err != nil {
			core.Bot.SendMessage(g.chatID, text)
		}
	} else {
		core.Bot.SendMessage(g.chatID, text)
	}
	wgEnd(g)
}

func wordGameHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()

	wgMu.Lock()
	if _, ok := wgGames[chatID]; ok {
		wgMu.Unlock()
		m.Reply("⚠️ A game is already running in this chat. Finish it first!")
		return tg.ErrEndGroup
	}
	g := &wordGame{
		chatID:  chatID,
		starter: m.SenderID(),
		entry:   wgPick(chatID),
		stage:   1,
	}
	wgGames[chatID] = g
	wgMu.Unlock()

	wgSendRound(g)
	return tg.ErrEndGroup
}

// wordGameEndHandler stops the running game. The player who started it, chat
// admins, the bot owner and sudo users may use it (anyone in DM).
func wordGameEndHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()

	wgMu.Lock()
	g := wgGames[chatID]
	wgMu.Unlock()
	if g == nil {
		m.Reply("ℹ️ No game is running in this chat.")
		return tg.ErrEndGroup
	}

	if !m.IsPrivate() && m.SenderID() != g.starter && !isOwnerOrSudo(m.SenderID()) {
		if ok, err := utils.IsChatAdmin(m.Client, chatID, m.SenderID()); err != nil || !ok {
			m.Reply("⚠️ Only the player who started the game or an admin can end it.")
			return tg.ErrEndGroup
		}
	}

	g.mu.Lock()
	g.gen++ // a late timer does nothing
	msg := g.msg
	g.msg = nil
	text := "🛑 <b>Game ended.</b>\n\n🎬 Movie: <b>" + html.EscapeString(g.entry.Movie) +
		"</b>\n🎵 Song: <b>" + html.EscapeString(g.entry.Song) +
		"</b>\n\nPlay again: /wordgame"
	g.mu.Unlock()

	if msg != nil {
		if _, err := msg.Edit(text, &tg.SendOptions{}); err != nil {
			m.Reply(text)
		}
	} else {
		m.Reply(text)
	}
	wgEnd(g)
	return tg.ErrEndGroup
}

// wordGuessHandler checks every plain text message of a chat that has a game.
func wordGuessHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()

	wgMu.Lock()
	g := wgGames[chatID]
	wgMu.Unlock()
	if g == nil {
		return nil
	}

	text := strings.TrimSpace(m.Text())
	if text == "" || strings.HasPrefix(text, "/") {
		return nil
	}

	g.mu.Lock()
	if g.msg == nil || wgNorm(text) != wgNorm(g.answer()) {
		g.mu.Unlock()
		return nil
	}
	if g.timer != nil {
		g.timer.Stop()
	}
	g.gen++ // closes this round, a late timer does nothing
	stage := g.stage
	answer := g.answer()
	points := 20
	if g.h3 {
		points = 10
	} else if g.h2 {
		points = 15
	}
	if left := wordGameTime - time.Since(g.started); left > 0 {
		points += int(left.Seconds()) / 3 // speed bonus, up to +10
	}
	oldMsg := g.msg
	g.msg = nil
	g.mu.Unlock()

	who := utils.MentionHTML(m.Sender)
	kind := "🎬 Movie"
	if stage == 2 {
		kind = "🎵 Song"
	}
	name := strings.TrimSpace(m.Sender.FirstName + " " + m.Sender.LastName)
	if name == "" {
		name = "Player"
	}
	if err := database.AddGameScore(chatID, m.SenderID(), name, points); err != nil {
		gologging.ErrorF("wordgame: score save failed: %v", err)
	}
	done := fmt.Sprintf("✅ %s: <b>%s</b>\n\n🏆 %s got it right!  <b>+%d pts</b>", kind, html.EscapeString(answer), who, points)

	if stage == 1 {
		if _, err := oldMsg.Edit(done, &tg.SendOptions{}); err != nil {
			m.Reply(done)
		}
		g.mu.Lock()
		g.stage = 2
		g.mu.Unlock()
		wgSendRound(g)
		return nil
	}

	done += "\n\nPlay again: /wordgame\n•  Top players: /leaderboard"
	if _, err := oldMsg.Edit(done, &tg.SendOptions{}); err != nil {
		m.Reply(done)
	}
	wgEnd(g)
	return nil
}

// wordHintCallback handles the Hint 2 / Hint 3 buttons.
func wordHintCallback(cb *tg.CallbackQuery) error {
	chatID := cb.ChannelID()

	wgMu.Lock()
	g := wgGames[chatID]
	wgMu.Unlock()
	if g == nil {
		cb.Answer("This game has already ended.", &tg.CallbackOptions{Alert: true})
		return tg.ErrEndGroup
	}

	data := cb.DataString()

	g.mu.Lock()
	if g.msg == nil {
		g.mu.Unlock()
		cb.Answer("")
		return tg.ErrEndGroup
	}
	if data == "wg:hint" {
		if !g.h2 {
			g.h2 = true
		} else {
			g.h3 = true
		}
	}
	text := g.text()
	mk := g.markup()
	msg := g.msg
	g.mu.Unlock()

	opts := &tg.SendOptions{}
	if mk != nil {
		opts.ReplyMarkup = mk
	}
	if _, err := msg.Edit(text, opts); err != nil {
		gologging.ErrorF("wordgame: hint edit failed: %v", err)
	}
	cb.Answer("")
	return tg.ErrEndGroup
}

// wgRank gives a BGMI-style title from the total points of a player.
func wgRank(points int) string {
	switch {
	case points >= 1000:
		return "Conqueror"
	case points >= 600:
		return "Legend"
	case points >= 350:
		return "Elite"
	case points >= 200:
		return "Pro"
	case points >= 100:
		return "Skilled"
	case points >= 40:
		return "Rookie"
	}
	return "Noob"
}

// leaderboardHandler shows the top 8 players of this chat.
func leaderboardHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()

	top, err := database.TopGameScores(chatID, 8)
	if err != nil {
		gologging.ErrorF("wordgame: leaderboard failed: %v", err)
		m.Reply("⚠️ Could not load the leaderboard right now.")
		return tg.ErrEndGroup
	}
	if len(top) == 0 {
		m.Reply("🏆 <b>Word Game Leaderboard</b>\n\nNo one has played yet. Start with /wordgame!")
		return tg.ErrEndGroup
	}

	marks := []string{"🥇", "🥈", "🥉", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣"}
	var sb strings.Builder
	sb.WriteString("🏆 <b>Word Game Leaderboard</b>\n\n")
	for i, p := range top {
		name := p.Name
		if name == "" {
			name = "Player"
		}
		fmt.Fprintf(&sb, "%s <b>%d.</b> %s <i>(%s)</i> — <b>%d</b> pts\n",
			marks[i], i+1, html.EscapeString(name), wgRank(p.Points), p.Points)
		if i == 2 && len(top) > 3 {
			sb.WriteString("\n")
		}
	}
	m.Reply(sb.String())
	return tg.ErrEndGroup
}
