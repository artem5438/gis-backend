package assistant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"gis-backend/auth"
	"gis-backend/models"

	"github.com/go-chi/chi/v5"
)

const refusal = "Извините, у меня нет этой информации."

type errorResponse struct {
	Error string `json:"error"`
}

type uploadResponse struct {
	ID     uint   `json:"id"`
	Title  string `json:"title"`
	Chunks int    `json:"chunks"`
}

type askRequest struct {
	Message string `json:"message"`
}

type gradeResponse struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Grade  string `json:"grade"`
}

type askResponse struct {
	Answer  string          `json:"answer"`
	Sources []string        `json:"sources,omitempty"`
	Map     *ParcelSpot     `json:"map,omitempty"`
	Grades  []gradeResponse `json:"grades"`
}

type titleRequest struct {
	Title string `json:"title"`
}

type statusResponse struct {
	Status string `json:"status"`
}

// Upload сохраняет документ в MinIO и записывает фрагменты с векторами.
// @Summary      Загрузка документа ассистента
// @Tags         ассистент
// @Accept       mpfd
// @Produce      json
// @Param        file  formData  file  true  "txt, md, docx, pdf, xlsx или csv, до 50 МБ"
// @Success      201   {object}  uploadResponse
// @Failure      400   {object}  errorResponse
// @Failure      403   {object}  errorResponse
// @Failure      502   {object}  errorResponse
// @Router       /api/assistant/documents [post]
func Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := requireRole(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "нужен файл не больше 50 МБ")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "нужен файл в поле file")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "файл не прочитался")
		return
	}
	text, err := readUpload(r.Context(), header.Filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	chunks, err := newModelClient().prepare(r.Context(), text)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if len(chunks) == 0 {
		writeError(w, http.StatusBadRequest, "в файле нет текста")
		return
	}
	title := titleFromFilename(header.Filename)
	key := "assistant/" + randomKey() + strings.ToLower(path.Ext(header.Filename))
	if err := putObject(r.Context(), key, data, header.Header.Get("Content-Type")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	document, err := saveDocument(r.Context(), title, key, user.ID, chunks)
	if err != nil {
		removeObject(r.Context(), key)
		writeError(w, http.StatusBadGateway, "фрагменты не записались")
		return
	}
	writeJSON(w, http.StatusCreated, uploadResponse{
		ID:     document.ID,
		Title:  document.Title,
		Chunks: len(chunks),
	})
}

// Documents перечисляет загруженные документы.
// @Summary      Список документов ассистента
// @Tags         ассистент
// @Produce      json
// @Success      200  {array}   storedDocument
// @Failure      403  {object}  errorResponse
// @Router       /api/assistant/documents [get]
func Documents(w http.ResponseWriter, r *http.Request) {
	user, ok := requireRole(w, r)
	if !ok {
		return
	}
	documents, err := listDocuments(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusBadGateway, "список документов не собрался")
		return
	}
	if documents == nil {
		documents = []storedDocument{}
	}
	writeJSON(w, http.StatusOK, documents)
}

// Ask ищет три ближайших фрагмента, оценивает их и отвечает только по годным.
// @Summary      Вопрос ассистенту
// @Tags         ассистент
// @Accept       json
// @Produce      json
// @Param        body  body      askRequest  true  "Вопрос"
// @Success      200   {object}  askResponse
// @Failure      400   {object}  errorResponse
// @Failure      502   {object}  errorResponse
// @Router       /api/assistant/ask [post]
func Ask(w http.ResponseWriter, r *http.Request) {
	var body askRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	question := strings.TrimSpace(body.Message)
	if question == "" {
		writeError(w, http.StatusBadRequest, "нужен вопрос в поле message")
		return
	}
	if reply, ok := courtesyReply(question); ok {
		writeJSON(w, http.StatusOK, askResponse{Answer: reply})
		return
	}
	if query, ok := matchParcelQuestion(question); ok {
		answer, spot, err := parcelAnswer(r.Context(), query)
		if err != nil {
			writeError(w, http.StatusBadGateway, "участок по району не нашёлся")
			return
		}
		writeJSON(w, http.StatusOK, askResponse{Answer: answer, Map: spot, Grades: []gradeResponse{}})
		return
	}
	client := newModelClient()
	vector, err := client.embedQuery(r.Context(), question)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	chunks, err := recallChunks(r.Context(), vector, question)
	if err != nil {
		writeError(w, http.StatusBadGateway, "поиск фрагментов не выполнился")
		return
	}
	if len(chunks) == 0 {
		writeJSON(w, http.StatusOK, askResponse{Answer: refusal, Grades: []gradeResponse{}})
		return
	}
	numbered := make([]foundChunk, len(chunks))
	titles := map[int]string{}
	for index, chunk := range chunks {
		number := index + 1
		numbered[index] = foundChunk{Number: number, Title: chunk.Title, Text: chunk.Text}
		titles[number] = chunk.Title
	}
	result, err := client.answer(r.Context(), question, numbered)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	grades := make([]gradeResponse, 0, len(result.Grades))
	for _, grade := range result.Grades {
		grades = append(grades, gradeResponse{
			Number: grade.Number,
			Title:  titles[grade.Number],
			Grade:  grade.Grade,
		})
	}
	answer := strings.TrimSpace(result.Answer)
	if answer == "" {
		answer = refusal
	}
	writeJSON(w, http.StatusOK, askResponse{Answer: answer, Sources: sourceTitles(answer, grades), Grades: grades})
}

func sourceTitles(answer string, grades []gradeResponse) []string {
	if answer == "" || answer == refusal {
		return nil
	}
	seen := map[string]bool{}
	var titles []string
	for _, grade := range grades {
		title := strings.TrimSpace(grade.Title)
		if grade.Grade != "годится" || title == "" || seen[title] {
			continue
		}
		seen[title] = true
		titles = append(titles, title)
	}
	return titles
}

// Rename меняет название документа. Ключ файла в хранилище остаётся прежним.
// @Summary      Новое название документа
// @Tags         ассистент
// @Accept       json
// @Produce      json
// @Param        id    path      int          true  "Номер документа"
// @Param        body  body      titleRequest true  "Название"
// @Success      200   {object}  statusResponse
// @Failure      400   {object}  errorResponse
// @Failure      403   {object}  errorResponse
// @Failure      404   {object}  errorResponse
// @Router       /api/assistant/documents/{id} [patch]
func Rename(w http.ResponseWriter, r *http.Request) {
	row, ok := editableDocument(w, r)
	if !ok {
		return
	}
	var body titleRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	title, err := cleanTitle(body.Title)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := renameDocument(r.Context(), row.ID, title); err != nil {
		writeStoredError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// Delete убирает документ, его фрагменты и объект в хранилище.
// @Summary      Удаление документа
// @Tags         ассистент
// @Produce      json
// @Param        id  path      int  true  "Номер документа"
// @Success      200  {object}  statusResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Router       /api/assistant/documents/{id} [delete]
func Delete(w http.ResponseWriter, r *http.Request) {
	row, ok := editableDocument(w, r)
	if !ok {
		return
	}
	if err := deleteObject(r.Context(), row.ObjectKey); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := removeDocument(r.Context(), row.ID); err != nil {
		writeStoredError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

func randomKey() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "document"
	}
	return hex.EncodeToString(buf[:])
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func requireRole(w http.ResponseWriter, r *http.Request) (models.User, bool) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return models.User{}, false
	}
	if user.Role == nil || user.Role.Code == "" {
		writeError(w, http.StatusForbidden, "документы откроются после назначения роли")
		return models.User{}, false
	}
	return user, true
}

func readUpload(ctx context.Context, name string, data []byte) (string, error) {
	if err := rejectUpload(name, len(data)); err != nil {
		return "", err
	}
	if strings.ToLower(path.Ext(name)) == ".pdf" {
		return newModelClient().extractPDF(ctx, data)
	}
	return textFromUpload(name, data)
}

func editableDocument(w http.ResponseWriter, r *http.Request) (documentRecord, bool) {
	user, ok := requireRole(w, r)
	if !ok {
		return documentRecord{}, false
	}
	id, err := documentID(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "документ не найден")
		return documentRecord{}, false
	}
	row, err := documentByID(r.Context(), id)
	if err != nil {
		writeStoredError(w, err)
		return documentRecord{}, false
	}
	if !canEdit(user, row.UploadedBy) {
		writeError(w, http.StatusForbidden, "этот документ может изменить его автор или модератор")
		return documentRecord{}, false
	}
	return row, true
}

func documentID(r *http.Request) (uint, error) {
	parsed, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || parsed == 0 {
		return 0, errNotFound
	}
	return uint(parsed), nil
}

func writeStoredError(w http.ResponseWriter, err error) {
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "документ не найден")
		return
	}
	writeError(w, http.StatusBadGateway, "документ не сохранился")
}
