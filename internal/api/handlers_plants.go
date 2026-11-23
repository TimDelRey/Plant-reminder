package api

import (
	"net/http"

	"plantcare/internal/plants"
)

func (a *API) updatePlant(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	var in plants.PlantPatch
	if err := decode(w, r, &in); err != nil {
		return err
	}
	p, err := a.d.Plants.UpdatePlant(r.Context(), userID(r), id, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (a *API) listPlantEvents(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	evs, err := a.d.Plants.ListEvents(r.Context(), userID(r), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, evs)
	return nil
}

func (a *API) createEvent(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	var in plants.EventInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	ev, err := a.d.Plants.CreateEvent(r.Context(), userID(r), id, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, ev)
	return nil
}

func (a *API) importCare(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Events []plants.EventInput `json:"events"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	evs, err := a.d.Plants.ImportCare(r.Context(), userID(r), id, in.Events)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, evs)
	return nil
}

func (a *API) careSuggestions(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	sug, err := a.d.Plants.SuggestCare(r.Context(), userID(r), id, intQuery(r, "species_id", 0))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, sug)
	return nil
}

func (a *API) listUpcoming(w http.ResponseWriter, r *http.Request) error {
	u, err := a.d.Plants.ListUpcoming(r.Context(), userID(r), intQuery(r, "limit", 6))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, u)
	return nil
}

func (a *API) updateEvent(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	var in plants.EventPatch
	if err := decode(w, r, &in); err != nil {
		return err
	}
	ev, err := a.d.Plants.UpdateEvent(r.Context(), userID(r), id, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, ev)
	return nil
}

func (a *API) deleteEvent(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	if err := a.d.Plants.DeleteEvent(r.Context(), userID(r), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) completeEvent(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	ev, err := a.d.Plants.CompleteEvent(r.Context(), userID(r), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, ev)
	return nil
}

func (a *API) searchReference(w http.ResponseWriter, r *http.Request) error {
	res, err := a.d.Plants.SearchReference(r.Context(), userID(r), r.URL.Query().Get("q"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}
