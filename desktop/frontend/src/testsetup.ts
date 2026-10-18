// Specs describe the Mac app unless they say otherwise: jsdom reports no platform of its own.
Object.defineProperty(navigator, "platform", { value: "MacIntel", configurable: true });
