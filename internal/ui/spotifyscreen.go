package ui

import (
	"context"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"alarmclock/internal/alarm"
	"alarmclock/internal/spotify"
)

// openSpotify opens the Spotify browse/play screen.
func (a *App) openSpotify() {
	a.cur = screenSpotify
	a.spotPick = false
	a.setSpotStatus("")
	if a.spot != nil && a.spot.Authorized() && a.spotTab == 1 {
		a.spotFetchPlaylists()
	}
}

// openSpotifyForPick opens the screen to choose a playlist for the current
// draft alarm rather than to play.
func (a *App) openSpotifyForPick() {
	a.cur = screenSpotify
	a.spotPick = true
	a.spotTab = 1
	a.setSpotStatus("")
	if a.spot != nil && a.spot.Authorized() {
		a.spotFetchPlaylists()
	}
}

func (a *App) spotFetchArtists(query string) {
	if a.spot == nil || query == "" {
		return
	}
	a.setSpotLoading()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := a.spot.SearchArtists(ctx, query, spotSearchLimit)
		a.spotMu.Lock()
		a.spotLoading = false
		if err != nil {
			a.spotErr = err.Error()
		} else {
			a.spotArtists = res
			a.spotErr = ""
		}
		a.spotMu.Unlock()
		a.refresh()
	}()
}

func (a *App) spotFetchPlaylists() {
	if a.spot == nil {
		return
	}
	a.setSpotLoading()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := a.spot.SavedPlaylists(ctx, maxSpotItems)
		a.spotMu.Lock()
		a.spotLoading = false
		if err != nil {
			a.spotErr = err.Error()
		} else {
			a.spotPlaylists = res
			a.spotErr = ""
		}
		a.spotMu.Unlock()
		a.refresh()
	}()
}

func (a *App) spotConnectStart() {
	if a.spot == nil {
		return
	}
	a.spotMu.Lock()
	if a.spotAuthorizing {
		a.spotMu.Unlock()
		return
	}
	a.spotAuthorizing = true
	a.spotStatus = "Autoriseer in de browser…"
	a.spotMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		err := a.spot.Authorize(ctx)
		a.spotMu.Lock()
		a.spotAuthorizing = false
		if err != nil {
			a.spotStatus = "Verbinden mislukt: " + err.Error()
		} else {
			a.spotStatus = "Verbonden met Spotify"
		}
		a.spotMu.Unlock()
		if err == nil {
			a.spotFetchPlaylists()
		}
		a.refresh()
	}()
}

func (a *App) spotPlay(uri, name string) {
	if a.spot == nil {
		return
	}
	a.setSpotStatus("Afspelen: " + name)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := a.spot.PlayOnDevice(ctx, a.spotDevice, uri, nil); err != nil {
			a.setSpotStatus("Afspelen mislukt: " + err.Error())
		}
		a.refresh()
	}()
}

func (a *App) pauseSpotify() {
	if a.spot == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = a.spot.Pause(ctx)
	}()
	a.setSpotStatus("Gepauzeerd")
}

func (a *App) setSpotLoading() {
	a.spotMu.Lock()
	a.spotLoading = true
	a.spotErr = ""
	a.spotMu.Unlock()
}

func (a *App) setSpotStatus(s string) {
	a.spotMu.Lock()
	a.spotStatus = s
	a.spotMu.Unlock()
}

func (a *App) refresh() {
	if a.invalidate != nil {
		a.invalidate()
	}
}

type spotState struct {
	artists     []spotify.Artist
	playlists   []spotify.Playlist
	loading     bool
	authorizing bool
	errMsg      string
	status      string
}

func (a *App) spotSnapshot() spotState {
	a.spotMu.Lock()
	defer a.spotMu.Unlock()
	return spotState{a.spotArtists, a.spotPlaylists, a.spotLoading, a.spotAuthorizing, a.spotErr, a.spotStatus}
}

func (a *App) layoutSpotify(gtx layout.Context) layout.Dimensions {
	title := "Spotify"
	if a.spotPick {
		title = "Kies afspeellijst"
	}
	if a.spotBack.Clicked(gtx) {
		if a.spotPick {
			a.spotPick = false
			a.cur = screenEdit
		} else {
			a.cur = screenHome
		}
	}

	inset := layout.UniformInset(unit.Dp(24))
	// Not configured / no client.
	if a.spot == nil || !a.spot.Configured() {
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return header(gtx, a.th, &a.spotBack, title) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.spotMessage(gtx, "Spotify is niet geconfigureerd. Stel een Client ID in (config of ALARMCLOCK_SPOTIFY_CLIENT_ID).")
				}),
			)
		})
	}

	st := a.spotSnapshot()

	// Not authorized: show connect.
	if !a.spot.Authorized() {
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if a.spotConnect.Clicked(gtx) {
				a.spotConnectStart()
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return header(gtx, a.th, &a.spotBack, title) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(32)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						b := material.Button(a.th, &a.spotConnect, "Verbind met Spotify")
						b.Background = Mocha.Green
						b.Color = Mocha.Crust
						b.TextSize = unit.Sp(28)
						b.CornerRadius = unit.Dp(16)
						b.Inset = layout.UniformInset(unit.Dp(22))
						return b.Layout(gtx)
					})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if st.status == "" {
						return layout.Dimensions{}
					}
					return a.spotMessage(gtx, st.status)
				}),
			)
		})
	}

	// Authorized: tabs (unless picking), search, list.
	if a.spotTabSearch.Clicked(gtx) {
		a.spotTab = 0
	}
	if a.spotTabLibrary.Clicked(gtx) {
		a.spotTab = 1
		if len(st.playlists) == 0 {
			a.spotFetchPlaylists()
		}
	}
	if a.spotSearchBtn.Clicked(gtx) {
		a.spotFetchArtists(strings.TrimSpace(a.spotQuery.Text()))
	}
	for {
		ev, ok := a.spotQuery.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			a.spotFetchArtists(strings.TrimSpace(a.spotQuery.Text()))
		}
	}
	if a.spotPause.Clicked(gtx) {
		a.pauseSpotify()
	}

	// Determine the active list and handle row taps.
	inLibrary := a.spotPick || a.spotTab == 1
	var n int
	if inLibrary {
		n = min(len(st.playlists), len(a.spotRows))
	} else {
		n = min(len(st.artists), len(a.spotRows))
	}
	for i := 0; i < n; i++ {
		if a.spotRows[i].Clicked(gtx) {
			if inLibrary {
				a.selectPlaylist(st.playlists[i])
			} else {
				a.spotPlay(st.artists[i].URI, st.artists[i].Name)
			}
		}
	}

	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutSpotifyHeader(gtx, title) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if inLibrary {
					return layout.Dimensions{}
				}
				return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, a.layoutSpotifySearch)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutSpotifyStatus(gtx, st, n)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(a.th, &a.spotList).Layout(gtx, n, func(gtx layout.Context, i int) layout.Dimensions {
					if inLibrary {
						return a.spotItemRow(gtx, i, st.playlists[i].Name, "Afspeellijst")
					}
					return a.spotItemRow(gtx, i, st.artists[i].Name, "Artiest")
				})
			}),
		)
	})
}

// selectPlaylist either stores the playlist on the draft alarm (pick mode) or
// starts playing it.
func (a *App) selectPlaylist(p spotify.Playlist) {
	if a.spotPick {
		a.draft.Sound.Kind = alarm.SoundSpotify
		a.draft.Sound.Ref = p.URI
		a.draft.Sound.Label = p.Name
		a.spotPick = false
		a.cur = screenEdit
		return
	}
	a.spotPlay(p.URI, p.Name)
}

// layoutSpotifyHeader is the header plus (when not picking) the tab toggle and
// a pause button.
func (a *App) layoutSpotifyHeader(gtx layout.Context, title string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return header(gtx, a.th, &a.spotBack, title) }),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.spotPick {
				return layout.Dimensions{}
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return segButton(gtx, a.th, &a.spotTabSearch, "Zoeken", a.spotTab == 0, Mocha.Green)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return segButton(gtx, a.th, &a.spotTabLibrary, "Bibliotheek", a.spotTab == 1, Mocha.Green)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					b := material.Button(a.th, &a.spotPause, "Pauze")
					b.Background = Mocha.Surface1
					b.Color = Mocha.Text
					b.TextSize = unit.Sp(20)
					b.CornerRadius = unit.Dp(10)
					b.Inset = layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(18), Right: unit.Dp(18)}
					return b.Layout(gtx)
				}),
			)
		}),
	)
}

func (a *App) layoutSpotifySearch(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return roundedPanel(gtx, Mocha.Surface0, unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(a.th, &a.spotQuery, "Zoek een artiest…")
					ed.TextSize = unit.Sp(24)
					ed.Color = Mocha.Text
					ed.HintColor = Mocha.Overlay0
					return ed.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b := material.Button(a.th, &a.spotSearchBtn, "Zoek")
			b.Background = Mocha.Green
			b.Color = Mocha.Crust
			b.TextSize = unit.Sp(24)
			b.CornerRadius = unit.Dp(12)
			b.Inset = layout.UniformInset(unit.Dp(18))
			return b.Layout(gtx)
		}),
	)
}

func (a *App) layoutSpotifyStatus(gtx layout.Context, st spotState, count int) layout.Dimensions {
	var msg string
	col := Mocha.Overlay1
	switch {
	case st.loading:
		msg, col = "Laden…", Mocha.Subtext0
	case st.errMsg != "":
		msg, col = "Fout: "+st.errMsg, Mocha.Red
	case st.status != "":
		msg, col = st.status, Mocha.Green
	case count == 0 && a.spotTab == 0 && !a.spotPick:
		msg = "Zoek een artiest om te beginnen"
	case count == 0:
		msg = "Niets gevonden"
	default:
		return layout.Dimensions{}
	}
	l := material.Label(a.th, unit.Sp(22), msg)
	l.Color = col
	l.MaxLines = 2
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, l.Layout)
}

func (a *App) spotItemRow(gtx layout.Context, i int, title, subtitle string) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.spotRows[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return roundedPanel(gtx, Mocha.Surface0, unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									l := material.Label(a.th, unit.Sp(26), title)
									l.Color = Mocha.Text
									l.MaxLines = 1
									return l.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									l := material.Label(a.th, unit.Sp(18), subtitle)
									l.Color = Mocha.Overlay1
									return l.Layout(gtx)
								}),
							)
						}),
					)
				})
			})
		})
	})
}

func (a *App) spotMessage(gtx layout.Context, msg string) layout.Dimensions {
	l := material.Label(a.th, unit.Sp(24), msg)
	l.Color = Mocha.Subtext0
	return l.Layout(gtx)
}
