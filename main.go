package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "template/docs" // substitua por seu módulo real

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB
var jwtSecret = []byte("sua-chave-secreta-supersegura")

type PageData struct {
	Title     string
	Error     string
	Success   string
	Email     string
	Msg       string
	Nome      string
	Telefone  string
	NFazenda  string
	Descricao string
	Latitude  string
	Longitude string
}

type User struct {
	ID       int
	Nome     string
	Email    string
	Telefone string
	Endereco string
}

// Adicione esta struct ao seu código
type DashboardData struct {
    PageData
    Imagens []string
}

// Handler para upload de imagens
func uploadImagemHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
        return
    }

    // Limite de 10MB para upload
    err := r.ParseMultipartForm(10 << 20)
    if err != nil {
        http.Error(w, "Arquivo muito grande (máximo 10MB)", http.StatusBadRequest)
        return
    }

    file, handler, err := r.FormFile("imagem")
    if err != nil {
        http.Error(w, "Erro ao obter arquivo", http.StatusBadRequest)
        return
    }
    defer file.Close()

    // Obter email do usuário logado
    email := getEmailDoUsuario(r)
    if email == "" {
        http.Error(w, "Usuário não autenticado", http.StatusUnauthorized)
        return
    }

    // Obter ID do usuário
    var userID int
    err = db.QueryRow("SELECT id FROM users WHERE email = ?", email).Scan(&userID)
    if err != nil {
        http.Error(w, "Erro ao identificar usuário", http.StatusInternalServerError)
        return
    }

    // Criar diretório de uploads se não existir
    uploadDir := "./static/uploads"
    if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
        os.MkdirAll(uploadDir, 0755)
    }

    // Gerar nome único para o arquivo
    ext := filepath.Ext(handler.Filename)
    uniqueFilename := fmt.Sprintf("%d_%d%s", userID, time.Now().UnixNano(), ext)
    filePath := filepath.Join(uploadDir, uniqueFilename)

    // Salvar arquivo
    dst, err := os.Create(filePath)
    if err != nil {
        http.Error(w, "Erro ao salvar arquivo", http.StatusInternalServerError)
        return
    }
    defer dst.Close()

    if _, err := io.Copy(dst, file); err != nil {
        http.Error(w, "Erro ao salvar arquivo", http.StatusInternalServerError)
        return
    }

    // Salvar referência no banco de dados
    _, err = db.Exec("INSERT INTO farm_images (user_id, filename) VALUES (?, ?)", userID, uniqueFilename)
    if err != nil {
        os.Remove(filePath) // Remove o arquivo se falhar ao salvar no banco
        http.Error(w, "Erro ao salvar referência da imagem", http.StatusInternalServerError)
        return
    }

    http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// Handler para deletar imagens
func deleteImagemHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
        return
    }

    filename := r.FormValue("filename")
    if filename == "" {
        http.Error(w, "Nome do arquivo não especificado", http.StatusBadRequest)
        return
    }

    // Verificar se o arquivo pertence ao usuário
    email := getEmailDoUsuario(r)
    var userID int
    err := db.QueryRow("SELECT id FROM users WHERE email = ?", email).Scan(&userID)
    if err != nil {
        http.Error(w, "Erro ao identificar usuário", http.StatusInternalServerError)
        return
    }

    // Verificar se a imagem pertence ao usuário
    var count int
    err = db.QueryRow("SELECT COUNT(*) FROM farm_images WHERE user_id = ? AND filename = ?", userID, filename).Scan(&count)
    if err != nil || count == 0 {
        http.Error(w, "Imagem não encontrada ou não pertence ao usuário", http.StatusNotFound)
        return
    }

    // Excluir do banco de dados
    _, err = db.Exec("DELETE FROM farm_images WHERE user_id = ? AND filename = ?", userID, filename)
    if err != nil {
        http.Error(w, "Erro ao excluir imagem do banco de dados", http.StatusInternalServerError)
        return
    }

    // Excluir arquivo
    filePath := filepath.Join("./static/uploads", filename)
    if err := os.Remove(filePath); err != nil {
        log.Println("Aviso: não foi possível excluir o arquivo:", err)
    }

    http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// Função para obter imagens do usuário
func getImagensDoUsuario(email string) ([]string, error) {
    var imagens []string

    rows, err := db.Query(`
        SELECT fi.filename 
        FROM farm_images fi
        JOIN users u ON fi.user_id = u.id
        WHERE u.email = ?
        ORDER BY fi.uploaded_at DESC
    `, email)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    for rows.Next() {
        var filename string
        if err := rows.Scan(&filename); err != nil {
            return nil, err
        }
        imagens = append(imagens, filename)
    }

    return imagens, nil
}

func initDB() {
	var err error
	db, err = sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/develop?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}
}

func gerarJWT(email string) (string, error) {
	claims := jwt.MapClaims{
		"email": email,
		"exp":   time.Now().Add(2 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func autenticarJWT(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("token")
		if err != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		tokenStr := cookie.Value
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func updateProducerInfo(db *sql.DB, data PageData) error {
	query := `
        UPDATE users
        SET
            nome_fazenda = ?,
            latitude = ?,
            longitude = ?,
            nome = ?,
            telefone = ?
        WHERE email = ?
    `
	_, err := db.Exec(query,
		data.NFazenda,  // nome_fazenda
		data.Latitude,  // latitude
		data.Longitude, // longitude
		data.Nome,      // nome
		data.Telefone,  // telefone
		data.Email,     // email no WHERE
	)
	return err
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	var user User

	query := `
		SELECT id, nome, telefone, endereco, cidade, estado, nome_fazenda
		FROM users
		ORDER BY id DESC
		LIMIT 1
	`
	err := db.QueryRow(query).Scan(
		&user.ID,
		&user.Nome,
		&user.Telefone,
		&user.Endereco,
	)
	if err != nil {
		http.Error(w, "Erro ao buscar produtor", http.StatusInternalServerError)
		return
	}

	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	err = tmpl.Execute(w, user)
	if err != nil {
		http.Error(w, "Erro ao renderizar a página", http.StatusInternalServerError)
	}
}



func atualizarProdutorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Erro ao ler formulário", http.StatusBadRequest)
		return
	}

	data := PageData{
		Nome:      r.FormValue("nome"),
		Email:     r.FormValue("email"),
		Telefone:  r.FormValue("telefone"),
		NFazenda:  r.FormValue("fazenda"),
		Descricao: r.FormValue("descricao"),
		// Latitude e Longitude ficam no outro formulário (mapa)
	}

	err = updateProducerInfo(db, data)
	if err != nil {
		data.Error = "Erro ao atualizar informações: " + err.Error()
		// Renderizar a página com erro (supondo que você tenha um template)
		renderPage(w, "dashboard.html", data)
		return
	}

	data.Success = "Informações atualizadas com sucesso!"
	// Renderizar a página com sucesso
	renderPage(w, "dashboard.html", data)
}

func getNomeDoProdutor(r *http.Request) string {
	cookie, err := r.Cookie("token")
	if err != nil {
		return ""
	}
	tokenStr := cookie.Value
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		email := claims["email"].(string)
		var nome string
		err := db.QueryRow("SELECT nome FROM users WHERE email = ?", email).Scan(&nome)
		if err == nil {
			return nome
		}
	}
	return ""
}

func gerarToken6Digitos() string {
	const charset = "0123456789"
	b := make([]byte, 6)
	rand.Read(b)
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

func enviarEmailComCodigo(destinatario, codigo string) error {
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"
	username := "jjooaaoo46@gmail.com"
	password := "mork fxry lwpi dkiw"

	from := username
	to := []string{destinatario}

	subject := "Seu código de verificação"
	body := "Seu código de verificação é: " + codigo
	message := []byte(fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body))

	auth := smtp.PlainAuth("", username, password, smtpHost)
	return smtp.SendMail(smtpHost+":"+smtpPort, auth, from, to, message)
}

func renderPage(w http.ResponseWriter, filename string, data PageData) {
	tmpl := template.Must(template.ParseFiles("src/" + filename))
	tmpl.Execute(w, data)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie := &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Secure:   false, // Altere para true em produção com HTTPS
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Páginas HTML

func cadastroPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "cadastro.html", PageData{Title: "Cadastro"})
}

func loginPage(w http.ResponseWriter, r *http.Request) {
	msg := r.URL.Query().Get("msg")
	var successMsg string
	if msg == "cadastro_sucesso" {
		successMsg = "Cadastro realizado com sucesso! Um código foi enviado para seu e-mail para confirmação."
	}
	renderPage(w, "login.html", PageData{Title: "Login", Success: successMsg})
}

func verificarTokenPage(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	renderPage(w, "verificar_token.html", PageData{Title: "Verificação", Email: email})
}

func dashboardPage(w http.ResponseWriter, r *http.Request) {
    nome := getNomeDoProdutor(r)
    email := getEmailDoUsuario(r)
    telefone := getTelefoneDoUsuario(r)
    nfazenda := getNomeFazendaDoUsuario(r)
    latitude := getLatitudeDoUsuario(r)
    longitude := getLongitudeDoUsuario(r)

    // Obter imagens do usuário
    imagens, err := getImagensDoUsuario(email)
    if err != nil {
        log.Println("Erro ao buscar imagens:", err)
        imagens = []string{} // Retorna array vazio em caso de erro
    }

    data := DashboardData{
        PageData: PageData{
            Title:     "Dashboard",
            Nome:      nome,
            Email:     email,
            Telefone:  telefone,
            NFazenda:  nfazenda,
            Longitude: longitude,
            Latitude:  latitude,
        },
        Imagens: imagens,
    }

    tmpl := template.Must(template.ParseFiles("src/dashboard.html"))
    tmpl.Execute(w, data)
}

func indexPage(w http.ResponseWriter, r *http.Request) {
	nome := getNomeDoProdutor(r)
	email := getEmailDoUsuario(r)

	var latitude, longitude string
	if email != "" {
		err := db.QueryRow("SELECT latitude, longitude FROM users WHERE email = ?", email).Scan(&latitude, &longitude)
		if err != nil {
			log.Println("Erro ao buscar localização:", err)
		}
	}

	data := struct {
		Title     string
		Nome      string
		Email     string
		Latitude  string
		Longitude string
	}{
		Title:     "Mapa do Produtor",
		Nome:      nome,
		Email:     email,
		Latitude:  latitude,
		Longitude: longitude,
	}

	tmpl := template.Must(template.ParseFiles("src/index.html"))
	tmpl.Execute(w, data)
}

func getEmailDoUsuario(r *http.Request) string {
	cookie, err := r.Cookie("token")
	if err != nil {
		return ""
	}
	tokenStr := cookie.Value
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims["email"].(string)
	}
	return ""
}

func getTelefoneDoUsuario(r *http.Request) string {
	cookie, err := r.Cookie("token")
	if err != nil {
		return ""
	}
	tokenStr := cookie.Value
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return ""
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		email := claims["email"].(string)
		var telefone string
		err := db.QueryRow("SELECT telefone FROM users WHERE email = ?", email).Scan(&telefone)
		if err == nil {
			return telefone
		}
	}
	return ""
}

func getNomeFazendaDoUsuario(r *http.Request) string {
	cookie, err := r.Cookie("token")
	if err != nil {
		return ""
	}

	tokenStr := cookie.Value
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return ""
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		email, ok := claims["email"].(string)
		if !ok {
			return ""
		}

		var nomeFazenda string
		err := db.QueryRow("SELECT nome_fazenda FROM users WHERE email = ?", email).Scan(&nomeFazenda)
		if err != nil {
			log.Println("Erro ao buscar nome da fazenda:", err)
			return ""
		}
		return nomeFazenda
	}

	return ""
}

func getLatitudeDoUsuario(r *http.Request) string {
	cookie, err := r.Cookie("token")
	if err != nil {
		log.Println("Token não encontrado:", err)
		return ""
	}

	tokenStr := cookie.Value
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		log.Println("Token JWT inválido:", err)
		return ""
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		log.Println("Erro ao extrair claims do token")
		return ""
	}

	email, ok := claims["email"].(string)
	if !ok {
		log.Println("Email não encontrado no token")
		return ""
	}

	var latitude string
	err = db.QueryRow("SELECT latitude FROM users WHERE email = ?", email).Scan(&latitude)
	if err != nil {
		log.Println("Erro ao buscar latitude do usuário:", err)
		return ""
	}

	return latitude
}


func getLongitudeDoUsuario(r *http.Request) string {
	cookie, err := r.Cookie("token")
	if err != nil {
		return ""
	}

	tokenStr := cookie.Value
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return ""
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		email, ok := claims["email"].(string)
		if !ok {
			return ""
		}

		var longitude string
		err := db.QueryRow("SELECT longitude FROM users WHERE email = ?", email).Scan(&longitude)
		if err != nil {
			log.Println("Erro ao buscar nome da fazenda:", err)
			return ""
		}
		return longitude
	}

	return ""
}

func successPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "sucesso.html", PageData{Title: "Sucesso"})
}

// API JSON

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Email string `json:"email"`
}

func apiLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, "Dados inválidos", http.StatusBadRequest)
		return
	}

	var hashedPassword string
	err = db.QueryRow("SELECT password FROM users WHERE email = ?", req.Email).Scan(&hashedPassword)
	if err != nil {
		http.Error(w, "Credenciais inválidas", http.StatusUnauthorized)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password))
	if err != nil {
		http.Error(w, "Credenciais inválidas", http.StatusUnauthorized)
		return
	}

	// Geração e envio do token 2FA
	token := gerarToken6Digitos()
	expira := time.Now().Add(5 * time.Minute)
	_, err = db.Exec("UPDATE users SET twofa_token = ?, twofa_expires = ? WHERE email = ?", token, expira, req.Email)
	if err != nil {
		http.Error(w, "Erro ao salvar token", http.StatusInternalServerError)
		return
	}

	err = enviarEmailComCodigo(req.Email, token)
	if err != nil {
		http.Error(w, "Erro ao enviar e-mail", http.StatusInternalServerError)
		return
	}

	resp := LoginResponse{Email: req.Email}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type VerifyTokenRequest struct {
	Email  string `json:"email"`
	Codigo string `json:"codigo"`
}

func apiVerificarTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req VerifyTokenRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.Email == "" || req.Codigo == "" {
		http.Error(w, "Dados inválidos", http.StatusBadRequest)
		return
	}

	var tokenDB string
	var expira time.Time
	err = db.QueryRow("SELECT twofa_token, twofa_expires FROM users WHERE email = ?", req.Email).Scan(&tokenDB, &expira)
	if err != nil {
		http.Error(w, "Usuário não encontrado", http.StatusBadRequest)
		return
	}

	// Remove espaços extras para evitar falhas de comparação
	codigo := strings.TrimSpace(req.Codigo)
	tokenDB = strings.TrimSpace(tokenDB)

	if codigo != tokenDB {
		log.Printf("Token inválido: recebido '%s', esperado '%s'\n", codigo, tokenDB)
		http.Error(w, "Código inválido", http.StatusBadRequest)
		return
	}

	if time.Now().After(expira) {
		http.Error(w, "Código expirado", http.StatusBadRequest)
		return
	}

	jwtToken, err := gerarJWT(req.Email)
	if err != nil {
		http.Error(w, "Erro ao gerar JWT", http.StatusInternalServerError)
		return
	}

	// Limpa o token do banco após validação
	_, _ = db.Exec("UPDATE users SET twofa_token=NULL, twofa_expires=NULL WHERE email=?", req.Email)

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    jwtToken,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   7200,
	})

	w.WriteHeader(http.StatusOK)
}


func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderPage(w, "cadastro.html", PageData{Title: "Cadastro"})
		return
	}

	err := r.ParseForm()
	if err != nil {
		log.Println("Erro ao parsear o formulário:", err)
		renderPage(w, "cadastro.html", PageData{Title: "Cadastro", Error: "Erro ao processar dados do formulário."})
		return
	}

	nome := r.FormValue("nome")
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	cpfCnpj := r.FormValue("cpf_cnpj")
	telefone := r.FormValue("telefone")
	nomeFazenda := r.FormValue("nome_fazenda")
	endereco := r.FormValue("endereco")
	cidade := r.FormValue("cidade")
	estado := r.FormValue("estado")
	cep := r.FormValue("cep")

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Println("Erro ao criptografar senha:", err)
		renderPage(w, "cadastro.html", PageData{Title: "Cadastro", Error: "Erro ao processar senha."})
		return
	}

	_, err = db.Exec(`INSERT INTO users 
		(nome, email, password, cpf_cnpj, telefone, nome_fazenda, endereco, cidade, estado, cep)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nome, email, string(hashedPassword), cpfCnpj, telefone, nomeFazenda, endereco, cidade, estado, cep)

	if err != nil {
		log.Println("Erro ao inserir:", err)
		renderPage(w, "cadastro.html", PageData{Title: "Cadastro", Error: "Erro ao cadastrar. Verifique os dados."})
		return
	}

	token := gerarToken6Digitos()
	expira := time.Now().Add(5 * time.Minute)
	_, err = db.Exec("UPDATE users SET twofa_token = ?, twofa_expires = ? WHERE email = ?", token, expira, email)
	if err != nil {
		log.Println("Erro ao salvar token:", err)
		renderPage(w, "cadastro.html", PageData{Title: "Cadastro", Error: "Erro ao salvar token"})
		return
	}
	err = enviarEmailComCodigo(email, token)
	if err != nil {
		log.Println("Erro ao enviar e-mail:", err)
		renderPage(w, "cadastro.html", PageData{Title: "Cadastro", Error: "Erro ao enviar e-mail"})
		return
	}

	http.Redirect(w, r, "/login?msg=cadastro_sucesso", http.StatusSeeOther)
}

func AtualizarLocalizacaoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Erro ao processar o formulário", http.StatusBadRequest)
		return
	}

	latitude := strings.TrimSpace(r.FormValue("latitude"))
	longitude := strings.TrimSpace(r.FormValue("longitude"))

	if latitude == "" || longitude == "" {
		http.Error(w, "Latitude ou longitude vazia", http.StatusBadRequest)
		return
	}

	// Extrair e-mail do token JWT
	cookie, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "Token ausente", http.StatusUnauthorized)
		return
	}
	tokenStr := cookie.Value
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		http.Error(w, "Token inválido", http.StatusUnauthorized)
		return
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, "Token malformado", http.StatusUnauthorized)
		return
	}
	email := claims["email"].(string)

	// Atualiza a localização no banco
	_, err = db.Exec("UPDATE users SET latitude = ?, longitude = ? WHERE email = ?", latitude, longitude, email)
	if err != nil {
		log.Println("Erro ao atualizar localização:", err)
		http.Error(w, "Erro ao atualizar localização no banco", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func main() {
	initDB()
	defer db.Close()

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Páginas HTML
	http.HandleFunc("/", indexPage)
	http.HandleFunc("/dashboard", autenticarJWT(dashboardPage))
	http.HandleFunc("/cadastro", cadastroPage)
	http.HandleFunc("/login", loginPage)
	http.HandleFunc("/verificar-token", verificarTokenPage)
	http.HandleFunc("/sucesso", autenticarJWT(successPage))
	http.HandleFunc("/logout", logoutHandler)

	// API - Autenticação
	http.HandleFunc("/delete-imagem", autenticarJWT(deleteImagemHandler))
	http.HandleFunc("/upload-imagem", autenticarJWT(uploadImagemHandler))
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/api/login", apiLoginHandler)
	http.HandleFunc("/api/verificar-token", apiVerificarTokenHandler)

	// Formulário protegido por JWT
	http.HandleFunc("/atualizar-produtor", autenticarJWT(atualizarProdutorHandler))
	http.HandleFunc("/atualizar-localizacao", autenticarJWT(AtualizarLocalizacaoHandler))

	log.Println("Servidor iniciado na porta :8080")
	http.ListenAndServe(":8080", nil)
}