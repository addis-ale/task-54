package handler

import (
	"bytes"
	"html/template"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type OccupancyHandler struct {
	admissions *service.AdmissionsService
}

func NewOccupancyHandler(admissions *service.AdmissionsService) *OccupancyHandler {
	return &OccupancyHandler{admissions: admissions}
}

type occupancyWardView struct {
	WardName string
	Beds     []domain.BedOccupancy
}

var occupancyFragmentTemplate = template.Must(template.New("occupancy_board").Funcs(template.FuncMap{
	"statusLabel": func(status string) string {
		switch status {
		case domain.BedStatusAvailable:
			return "Available"
		case domain.BedStatusOccupied:
			return "Occupied"
		case domain.BedStatusCleaning:
			return "Cleaning"
		default:
			return "Maintenance"
		}
	},
}).Parse(`
<section id="occupancy-board-fragment" class="occupancy-board" hx-swap-oob="true">
  {{- if not .Wards }}
  <div class="occupancy-empty">No beds configured yet.</div>
  {{- end }}
  {{- range .Wards }}
  <div class="occupancy-ward">
    <h3 class="occupancy-ward-title">{{ .WardName }}</h3>
    <div class="occupancy-grid">
      {{- range .Beds }}
      <article class="bed-card bed-status-{{ .Status }}" data-bed-id="{{ .BedID }}" data-version="{{ .Version }}">
        <header class="bed-card-header">
          <span class="bed-code">{{ .BedCode }}</span>
          <span class="bed-status">{{ statusLabel .Status }}</span>
        </header>
        <div class="bed-card-body">
          {{- if .PatientName }}
          <p class="bed-patient">{{ .PatientName }}</p>
          {{- with .PatientID }}
          <button class="btn-sm" hx-get="/ui/service-delivery/patient/{{ . }}" hx-target="#panel" title="View service delivery">Drill Down</button>
          {{- end }}
          {{- else if eq .Status "occupied" }}
          <p class="bed-patient">Patient not linked</p>
          {{- else if eq .Status "cleaning" }}
          <p class="bed-patient">Needs cleaning turnover</p>
          {{- else }}
          <p class="bed-patient">Ready for assignment</p>
          {{- end }}
        </div>
      </article>
      {{- end }}
    </div>
  </div>
  {{- end }}
</section>
`))

func (h *OccupancyHandler) Board(c *fiber.Ctx) error {
	items, err := h.admissions.OccupancyBoard(c.UserContext())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to render occupancy board")
	}

	wards := make([]occupancyWardView, 0)
	wardIndex := make(map[int64]int)
	for _, item := range items {
		idx, ok := wardIndex[item.WardID]
		if !ok {
			idx = len(wards)
			wardIndex[item.WardID] = idx
			wards = append(wards, occupancyWardView{WardName: item.WardName, Beds: make([]domain.BedOccupancy, 0)})
		}
		wards[idx].Beds = append(wards[idx].Beds, item)
	}

	var out bytes.Buffer
	if err := occupancyFragmentTemplate.Execute(&out, fiber.Map{"Wards": wards}); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to execute occupancy template")
	}

	c.Type("html", "utf-8")
	return c.Send(out.Bytes())
}
