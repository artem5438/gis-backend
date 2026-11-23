package assistant

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
)

const refusal = "В документах этого нет."

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
	Answer string          `json:"answer"`
	Grades []gradeResponse `json:"grades"`
}

// Upload сохраняет документ в MinIO и записывает фрагменты с векторами.
// @Summary      Загрузка документа ассистента
// @Tags         ассистент
// @Accept       mpfd
// @Produce      json
// @Param        file  formData  file  true  "Текст .txt, .md или .docx"
// @Success      201   {object}  uploadResponse
// @Failure      400   {object}  errorResponse
// @Failure      502   {object}  errorResponse
// @Router       /api/assistant/documents [post]
func Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "нужен файл не больше 8 МБ")
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
	text, err := textFromUpload(header.Filename, data)
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
	title := strings.TrimSpace(path.Base(header.Filename))
	key := "assistant/" + randomKey() + strings.ToLower(path.Ext(header.Filename))
	if err := putObject(r.Context(), key, data, header.Header.Get("Content-Type")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	document, err := saveDocument(r.Context(), title, key, chunks)
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
// @Router       /api/assistant/documents [get]
func Documents(w http.ResponseWriter, r *http.Request) {
	documents, err := listDocuments(r.Context())
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
	client := newModelClient()
	vector, err := client.embedQuery(r.Context(), question)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	chunks, err := nearestChunks(r.Context(), vector)
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
	writeJSON(w, http.StatusOK, askResponse{Answer: answer, Grades: grades})
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
