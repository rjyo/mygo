// Rust-native calls a Rust crate from native UI. The crate is in
// adder/rust, and adder/mygo-plugin.json tells the CLI to build it with
// cargo: mygo dev and mygo build put the library into the app, and package
// ffi binds its functions. Run it with
//
//	go run ./cmd/mygo dev examples/rust-native
//
// or, after building the library for this machine,
//
//	go run ./cmd/mygo native build -target host examples/rust-native
//	go run ./examples/rust-native
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/examples/rust-native/adder"
	"github.com/egoist/mygo/ui"
)

type state struct {
	n   uint32
	err error
}

func (s *state) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		if s.err != nil {
			ui.Text(c, s.err.Error()).TextColor(t.Danger).MaxWidth(420)
			return
		}
		ui.Text(c, adder.Version()).FontSize(12).TextColor(t.TextMuted)
		ui.Textf(c, "fibonacci(%d) = %d", s.n, adder.Fibonacci(s.n)).FontSize(28).Bold()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key("less"), "−").Label("Previous").Width(44).Disabled(s.n == 0).Clicked() {
				s.n--
			}
			if ui.PrimaryButton(c.Key("more"), "+").Label("Next").Width(44).Disabled(s.n == 93).Clicked() {
				s.n++
			}
		})
		ui.Textf(c, "add(40, 2) = %d · %d vowels in “MyGo calls Rust”", adder.Add(40, 2), adder.CountVowels("MyGo calls Rust")).
			FontSize(12).TextColor(t.TextMuted)
	})
}

func main() {
	s := &state{n: 10, err: adder.Load()}
	if s.err != nil {
		log.Print(s.err)
	}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Rust",
			Width:   420,
			Height:  260,
			Content: ui.View(s.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
