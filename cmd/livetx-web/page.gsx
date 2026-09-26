package main

import (
	"github.com/livetx/livetx/internal/audio"
	"github.com/livetx/livetx/ui"
)

// Page is the full LiveTX web UI, rendered server-side with gsxui components.
// Interactivity (device start/stop, live transcript over WebSocket) is driven
// by /app.js; styling comes from the compiled /app.css.
component Page(devices []audio.Device) {
	<html lang="en" class="dark">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>LiveTX</title>
			<link rel="stylesheet" href="/app.css"/>
		</head>
		<body class="overflow-hidden bg-background text-foreground flex flex-col" style="height:100vh">
			<header class="flex shrink-0 flex-wrap items-center gap-3 border-b border-border bg-card px-4 py-3">
				<div class="mr-auto flex items-center gap-2 font-semibold tracking-wide">
					<span id="dot" class="inline-block size-2 rounded-full bg-muted-foreground"></span>
					LiveTX
				</div>

				<ui.Label for="device">Device</ui.Label>
				<ui.NativeSelect id="device" class="min-w-56">
					{ if len(devices) == 0 {
						<ui.NativeSelectOption value="">No devices found</ui.NativeSelectOption>
					} else {
						{ for _, d := range devices {
							<ui.NativeSelectOption value={d.ID}>{d.Name}</ui.NativeSelectOption>
						} }
					} }
				</ui.NativeSelect>

				<ui.Label for="langIn">In</ui.Label>
				<input id="langIn" value="auto"
					class="h-8 w-16 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"/>
				<ui.Label for="langOut">Out</ui.Label>
				<input id="langOut" value="en"
					class="h-8 w-16 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"/>

				<ui.Button id="toggle">Start</ui.Button>
			</header>

			<main class="flex-1 overflow-y-auto px-5 py-6" style="min-height:0">
				<div id="transcript" class="mx-auto max-w-3xl space-y-2.5">
					<p class="mt-10 text-center text-muted-foreground">Pick a device and press Start.</p>
				</div>
			</main>

			<footer class="flex shrink-0 items-center gap-3 border-t border-border bg-card px-4 py-2">
				<ui.Badge id="status" variant="secondary">Ready</ui.Badge>
				<ui.Button id="autoscroll" size="sm" variant="outline" class="ml-auto">Auto-scroll: On</ui.Button>
			</footer>

			<script src="/app.js"></script>
		</body>
	</html>
}
