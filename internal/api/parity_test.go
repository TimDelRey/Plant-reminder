package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"

	"plantcare/internal/auth"
	"plantcare/internal/cities"
	"plantcare/internal/config"
	"plantcare/internal/plants"
	"plantcare/internal/sender"
)

// Паритет «действие ↔ ручка API ↔ документация». Если создали действие в домене, но не создали ручку
// (или не описали её в openapi.yaml) — тест падает. Описание правила: dev-план, раздел 8.1.

// каталоги internal/, которые не содержат действий пользователя
var skipDirs = map[string]bool{
	"api": true, "apperr": true, "config": true, "db": true, "mailer": true,
	"sender/max": true, "sender/vk": true, "sender/telegram": true, // адаптеры платформ
}

type action struct{ key, file string }

// domainActions находит (через AST) все экспортируемые методы экспортируемых типов в доменных пакетах.
func domainActions(t *testing.T) []action {
	t.Helper()
	root := filepath.Join("..") // internal/
	var out []action
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if skipDirs[rel] {
			return filepath.SkipDir
		}
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, p, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
		if err != nil {
			return err
		}
		for _, pkg := range pkgs {
			for fname, f := range pkg.Files {
				for _, decl := range f.Decls {
					fd, ok := decl.(*ast.FuncDecl)
					if !ok || fd.Recv == nil || !fd.Name.IsExported() {
						continue
					}
					typ := recvType(fd.Recv.List[0].Type)
					if typ == "" || !ast.IsExported(typ) {
						continue
					}
					out = append(out, action{key: pkg.Name + "." + typ + "." + fd.Name.Name, file: fname})
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func recvType(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.StarExpr:
		return recvType(v.X)
	case *ast.Ident:
		return v.Name
	case *ast.IndexExpr:
		return recvType(v.X)
	}
	return ""
}

func internalOnly(key string) bool {
	if _, ok := InternalOnly[key]; ok {
		return true
	}
	i := strings.LastIndex(key, ".")
	_, ok := InternalOnly[key[:i]+".*"]
	return ok
}

func testRouter() http.Handler {
	return NewRouter(Deps{
		Cfg: &config.Config{}, Auth: &auth.Service{}, Plants: &plants.Service{},
		Cities: &cities.Service{}, Linking: &sender.Linking{},
	})
}

func routerRoutes(t *testing.T, r http.Handler) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	err := chi.Walk(r.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/") // chi добавляет завершающий слэш в группах
		if route == "" || strings.HasSuffix(route, "/*") {
			return nil // статика Swagger UI
		}
		got[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func loadSpec(t *testing.T) (*openapi3.T, map[string]*openapi3.Operation) {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(OpenAPISpec)
	if err != nil {
		t.Fatalf("openapi.yaml не разбирается: %v", err)
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Fatalf("openapi.yaml невалидна: %v", err)
	}
	ops := map[string]*openapi3.Operation{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			ops[method+" "+path] = op
		}
	}
	return doc, ops
}

func sortedKeys[V any](m map[string]V) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}

// 1. Каждое действие домена имеет ручку в API (реестр) либо явно помечено как внутреннее.
func TestEveryDomainActionHasAPIRoute(t *testing.T) {
	for _, a := range domainActions(t) {
		if _, ok := Registry[a.key]; ok {
			continue
		}
		if internalOnly(a.key) {
			continue
		}
		t.Errorf("действие %s (%s) не имеет ручки в API: добавьте ручку, опишите её в openapi.yaml и внесите в Registry "+
			"(либо, если это не действие пользователя, — в InternalOnly с причиной)", a.key, a.file)
	}
	// и обратно: записи реестра не должны ссылаться на несуществующие действия
	have := map[string]bool{}
	for _, a := range domainActions(t) {
		have[a.key] = true
	}
	for key := range Registry {
		if !have[key] {
			t.Errorf("Registry содержит %s, но такого действия в домене нет", key)
		}
	}
	for key, why := range InternalOnly {
		if strings.TrimSpace(why) == "" {
			t.Errorf("InternalOnly[%s]: причина обязательна", key)
		}
	}
}

// 2. Каждое действие из реестра реально смонтировано в роутере.
func TestRegistryRoutesExistInRouter(t *testing.T) {
	routes := routerRoutes(t, testRouter())
	for key, rt := range Registry {
		if !routes[rt.Method+" "+rt.Path] {
			t.Errorf("действие %s: маршрута %s %s нет в роутере", key, rt.Method, rt.Path)
		}
	}
}

// 3 и 4. Роутер и openapi.yaml описывают один и тот же набор операций.
func TestRouterAndSpecAreInSync(t *testing.T) {
	_, ops := loadSpec(t)
	routes := routerRoutes(t, testRouter())
	for _, r := range sortedKeys(routes) {
		if _, ok := ops[strings.ToUpper(r[:strings.Index(r, " ")])+r[strings.Index(r, " "):]]; !ok {
			t.Errorf("ручка %s есть в роутере, но не описана в openapi.yaml", r)
		}
	}
	for _, o := range sortedKeys(ops) {
		if !routes[o] {
			t.Errorf("операция %s описана в openapi.yaml, но маршрута нет в роутере (документация устарела)", o)
		}
	}
}

// 5. Каждая операция в спеке описана достаточно полно.
func TestSpecOperationsAreDocumented(t *testing.T) {
	_, ops := loadSpec(t)
	for _, name := range sortedKeys(ops) {
		op := ops[name]
		if strings.TrimSpace(op.Summary) == "" {
			t.Errorf("%s: нет summary", name)
		}
		if op.OperationID == "" {
			t.Errorf("%s: нет operationId", name)
		}
		hasSuccess := false
		for code, resp := range op.Responses.Map() {
			if !strings.HasPrefix(code, "2") {
				continue
			}
			hasSuccess = true
			if code != "204" && code != "202" && (resp.Value == nil || len(resp.Value.Content) == 0) {
				t.Errorf("%s: у ответа %s нет описания схемы (content)", name, code)
			}
		}
		if !hasSuccess {
			t.Errorf("%s: не описан успешный ответ 2xx", name)
		}
		path := name[strings.Index(name, " ")+1:]
		if !strings.HasPrefix(path, "/webhooks/") {
			continue
		}
		tagged := false
		for _, tg := range op.Tags {
			tagged = tagged || tg == "internal"
		}
		if !tagged {
			t.Errorf("%s: вебхук должен иметь тег internal", name)
		}
	}
}

// 6. Защита: всё, что не помечено публичным, без авторизации отдаёт 401; публичное — нет.
func TestAuthRequirementMatchesRouter(t *testing.T) {
	_, ops := loadSpec(t)
	r := testRouter()
	for _, name := range sortedKeys(ops) {
		op := ops[name]
		method, path := name[:strings.Index(name, " ")], name[strings.Index(name, " ")+1:]
		public := op.Security != nil && len(*op.Security) == 0
		isPublicPath := false
		for _, pre := range PublicPrefixes {
			isPublicPath = isPublicPath || strings.HasPrefix(path, pre)
		}
		if public != isPublicPath {
			t.Errorf("%s: публичность в спеке (security: []) = %v не совпадает со списком PublicPrefixes = %v", name, public, isPublicPath)
		}
		if public {
			continue
		}
		url := strings.NewReplacer("{id}", "1", "{channel}", "telegram").Replace(path)
		req := httptest.NewRequest(method, url, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s без авторизации вернул %d, ожидался 401", name, rec.Code)
		}
	}
}
