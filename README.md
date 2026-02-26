# Moran-Go

Uma aplicação web em Go para gerenciamento de informações de produtores rurais, com autenticação segura, 2FA e rastreamento de localização.

## 🚀 Funcionalidades

- **Autenticação de Usuário**: Sistema seguro de cadastro e login usando Bcrypt para hash de senhas.
- **Autenticação de Dois Fatores (2FA)**: Códigos de verificação via e-mail para maior segurança.
- **Autorização JWT**: Gerenciamento de sessão usando JSON Web Tokens (JWT) armazenados em cookies seguros.
- **Gerenciamento de Perfil**: Atualização de informações do produtor, incluindo nome, nome da fazenda e detalhes de contato.
- **Rastreamento de Localização**: Armazenamento e atualização de coordenadas da fazenda (latitude e longitude) com suporte a mapa interativo.
- **Galeria de Imagens**: Upload e gerenciamento de imagens relacionadas à fazenda.
- **Documentação da API**: Documentação Swagger integrada para os endpoints da API.

## 🛠️ Tecnologias

- **Backend**: Go (Golang)
- **Banco de Dados**: MySQL
- **Frontend**: Templates HTML (Go `html/template`)
- **Segurança**: JWT (`golang-jwt/jwt/v5`), Bcrypt (`golang.org/x/crypto/bcrypt`)
- **Docs da API**: Swagger (`github.com/swaggo/swag`)
- **Comunicação**: SMTP para envio de e-mails de 2FA.

## 📂 Estrutura do Projeto

- `main.go`: Ponto de entrada e lógica principal do backend (handlers, middleware, inicialização do BD).
- `src/`: Templates HTML para o frontend.
- `static/`: Ativos estáticos (imagens, CSS, JS) e uploads de usuários.
- `docs/`: Documentação Swagger gerada automaticamente.
- `users.sql`: Script SQL para inicializar o esquema do banco de dados.
- `go.mod`: Dependências do módulo Go.

## ⚙️ Configuração e Instalação

### Pré-requisitos

- [Go](https://golang.org/doc/install) (versão 1.24.0 ou superior)
- [MySQL Server](https://dev.mysql.com/downloads/installer/)

### 1. Configuração do Banco de Dados

1. Crie um banco de dados chamado `develop`.
2. Execute o script `users.sql` para criar as tabelas necessárias:
   ```bash
   mysql -u root -p develop < users.sql
   ```
3. Atualize a string de conexão do banco de dados no `main.go` (se suas credenciais forem diferentes):
   ```go
   db, err = sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/develop?parseTime=true")
   ```

### 2. Configuração de E-mail (2FA)

A aplicação usa o servidor SMTP do Gmail para enviar códigos 2FA. Atualize estas variáveis no `main.go` com suas credenciais:
```go
username := "seu-email@gmail.com"
password := "sua-senha-de-app"
```

### 3. Executando a Aplicação

Instale as dependências:
```bash
go mod tidy
```

Inicie o servidor:
```bash
go run main.go
```

O servidor iniciará em `http://localhost:8080`.

## 📖 Documentação da API

O projeto inclui documentação Swagger. Após executar a aplicação, você pode encontrá-la em (verifique a rota):
- `http://localhost:8080/swagger/index.html` (Verifique a pasta `docs/` para detalhes de integração).

## 📄 Licença

Este projeto está licenciado sob a [MIT License](LICENSE).
