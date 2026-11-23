package plantinfo

import "strings"

// ruToEn — русские названия распространённых растений → поисковый запрос для Perenual (он понимает английские
// и латинские названия). Всё, чего здесь нет, ищется как введено (латиница работает) и иначе уходит в общий набор.
var ruToEn = map[string]string{
	"слива": "plum", "груша": "pear", "яблоня": "apple tree", "вишня": "cherry", "черешня": "sweet cherry",
	"абрикос": "apricot", "персик": "peach", "айва": "quince", "малина": "raspberry", "смородина": "currant",
	"крыжовник": "gooseberry", "виноград": "grape", "клубника": "strawberry", "земляника": "strawberry",
	"роза": "rose", "пион": "peony", "гортензия": "hydrangea", "сирень": "lilac", "жасмин": "jasmine",
	"тюльпан": "tulip", "нарцисс": "narcissus", "лаванда": "lavender", "хоста": "hosta", "ирис": "iris",
	"томат": "tomato", "помидор": "tomato", "огурец": "cucumber", "перец": "pepper", "баклажан": "eggplant",
	"картофель": "potato", "морковь": "carrot", "капуста": "cabbage", "базилик": "basil", "мята": "mint",
	"петрушка": "parsley", "укроп": "dill", "розмарин": "rosemary", "тимьян": "thyme",
	"фикус": "ficus", "фикус бенджамина": "ficus benjamina", "монстера": "monstera", "орхидея": "orchid",
	"фаленопсис": "phalaenopsis", "герань": "geranium", "пеларгония": "pelargonium", "кактус": "cactus",
	"алоэ": "aloe", "алоэ вера": "aloe vera", "суккулент": "succulent", "эхеверия": "echeveria",
	"замиокулькас": "zamioculcas", "драцена": "dracaena", "сансевиерия": "sansevieria", "щучий хвост": "sansevieria",
	"спатифиллум": "peace lily", "хлорофитум": "spider plant", "бегония": "begonia", "фиалка": "african violet",
	"сенполия": "african violet", "азалия": "azalea", "гибискус": "hibiscus", "лимон": "lemon", "апельсин": "orange",
	"мандарин": "mandarin", "папоротник": "fern", "нефролепис": "boston fern", "калатея": "calathea",
	"маранта": "maranta", "антуриум": "anthurium", "циперус": "cyperus", "пальма": "palm", "бамбук": "bamboo",
	"плющ": "ivy", "хойя": "hoya", "каланхоэ": "kalanchoe", "толстянка": "jade plant", "денежное дерево": "jade plant",
	"туя": "thuja", "ель": "spruce", "сосна": "pine", "берёза": "birch", "береза": "birch", "дуб": "oak", "клён": "maple",
}

// query — поисковый запрос к справочнику, полученный из названия растения.
type query struct {
	Q       string // что искать (английское/латинское)
	Partial bool   // найдено только по первому слову названия (сорт/уточнение отброшены)
	Word    string // слово, по которому искали при Partial
}

// queries возвращает поисковые запросы для названия растения (нормализованные), от более точного к менее.
func queries(name string) []query {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.Join(strings.Fields(strings.ReplaceAll(n, "ё", "е")), " ")
	if n == "" {
		return nil
	}
	lookup := func(k string) (string, bool) {
		if en, ok := ruToEn[k]; ok {
			return en, true
		}
		en, ok := ruToEn[strings.ReplaceAll(k, "е", "ё")]
		return en, ok
	}
	if isLatin(n) {
		return []query{{Q: n}}
	}
	if en, ok := lookup(n); ok {
		return []query{{Q: en}}
	}
	// кириллица без полного перевода: пробуем первое слово («слива бербанк» → «plum», уточнение «бербанк» теряется)
	if first := strings.Fields(n)[0]; first != n {
		if en, ok := lookup(first); ok {
			return []query{{Q: en, Partial: true, Word: first}}
		}
	}
	return nil
}

func isLatin(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
