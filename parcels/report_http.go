package parcels

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gis-backend/auth"
	"gis-backend/models"

	"github.com/go-chi/chi/v5"
)

type reportDraft struct {
	ParcelID uint           `json:"parcel_id"`
	Body     reportDocument `json:"body"`
}

type reportSaveRequest struct {
	ID         uint              `json:"id"`
	ParcelID   uint              `json:"parcel_id"`
	Number     string            `json:"number"`
	Format     string            `json:"format"`
	Score      bool              `json:"score"`
	Quarter    bool              `json:"quarter"`
	Charts     bool              `json:"charts"`
	ChartKinds map[string]string `json:"chart_kinds"`
}

func ReportDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := reportUser(w, r)
	if !ok {
		return
	}
	id, number, ok := reportTarget(w, r.URL.Query().Get("parcel"), r.URL.Query().Get("number"))
	if !ok {
		return
	}
	doc, card, err := buildReport(id, number, user.FullName, time.Now())
	if err != nil {
		writeReportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reportDraft{ParcelID: card.ID, Body: doc})
}

func ReportSave(w http.ResponseWriter, r *http.Request) {
	user, ok := reportUser(w, r)
	if !ok {
		return
	}
	var body reportSaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	format := strings.ToLower(strings.TrimSpace(body.Format))
	if format != "pdf" && format != "docx" {
		writeError(w, http.StatusBadRequest, "нужен формат pdf или docx")
		return
	}
	choices := reportChoices{Score: body.Score, Quarter: body.Quarter, Charts: body.Charts && body.Quarter}
	if body.ID != 0 {
		saved, err := updateReport(user.ID, body.ID, format, choices, body.ChartKinds)
		if err != nil {
			writeReportError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, saved)
		return
	}
	id, number, ok := reportTarget(w, strconv.FormatUint(uint64(body.ParcelID), 10), body.Number)
	if !ok {
		return
	}
	if body.ParcelID == 0 {
		id = 0
	}
	doc, card, err := buildReport(id, number, user.FullName, time.Now())
	if err != nil {
		writeReportError(w, err)
		return
	}
	applyChartKinds(&doc, body.ChartKinds)
	saved, err := insertReport(user, format, card, choices, doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "отчёт не сохранился")
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func ReportList(w http.ResponseWriter, r *http.Request) {
	user, ok := reportUser(w, r)
	if !ok {
		return
	}
	rows, err := listReports(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "список отчётов не собрался")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func ReportOne(w http.ResponseWriter, r *http.Request) {
	user, ok := reportUser(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	row, _, err := reportByID(user.ID, id)
	if err == errParcelMissing {
		writeError(w, http.StatusNotFound, "отчёт не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "отчёт не собрался")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func ReportFile(w http.ResponseWriter, r *http.Request) {
	user, ok := reportUser(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	row, doc, err := reportByID(user.ID, id)
	if err == errParcelMissing {
		writeError(w, http.StatusNotFound, "отчёт не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "отчёт не собрался")
		return
	}
	var data []byte
	var contentType string
	if row.Format == "docx" {
		data, err = renderDocx(doc, row.choices())
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	} else {
		data, err = renderPDF(doc, row.choices())
		contentType = "application/pdf"
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "файл отчёта не собрался")
		return
	}
	name := "otchet-" + strings.NewReplacer(":", "-", "/", "-", "\\", "-").Replace(row.Number) + "." + row.Format
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"; filename*=UTF-8''`+url.PathEscape(name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func reportUser(w http.ResponseWriter, r *http.Request) (models.User, bool) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return models.User{}, false
	}
	if user.VerificationStatus != models.StatusVerified {
		writeError(w, http.StatusForbidden, "отчёт доступен после проверки")
		return models.User{}, false
	}
	return user, true
}

func reportTarget(w http.ResponseWriter, parcel, number string) (uint, string, bool) {
	parcel = strings.TrimSpace(parcel)
	number = strings.TrimSpace(number)
	if parcel != "" && parcel != "0" {
		parsed, err := strconv.ParseUint(parcel, 10, 64)
		if err != nil || parsed == 0 {
			writeError(w, http.StatusBadRequest, "участок не найден")
			return 0, "", false
		}
		return uint(parsed), "", true
	}
	if number == "" {
		writeError(w, http.StatusBadRequest, "нужен участок")
		return 0, "", false
	}
	return 0, number, true
}

func reportID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	parsed, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || parsed == 0 {
		writeError(w, http.StatusNotFound, "отчёт не найден")
		return 0, false
	}
	return uint(parsed), true
}

func writeReportError(w http.ResponseWriter, err error) {
	if err == errParcelMissing {
		writeError(w, http.StatusNotFound, "участок не найден")
		return
	}
	writeError(w, http.StatusInternalServerError, "отчёт не собрался")
}
