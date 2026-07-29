package agent

import "math/rand/v2"

// Cold-start lines (first turn after gantry restarts). Keep short — Telegram
// status bubble, not a monologue.
var spinupColdNotes = []string{
	"⏳ spinning up — the first reply after a restart takes longer",
	"⏳ cold start — waking the weights from their nap",
	"⏳ same thing we do every night… warm the cache. Then: take over the world",
	"⏳ Pinky: \"What are we doing tonight?\" Brain: \"Loading the model.\"",
	"⏳ prompt cache is empty — making tea for the tokens",
	"⏳ \"I think, therefore I… hang on, still booting\"",
	"⏳ loading the brain into RAM… dignity optional",
	"⏳ silicon stretch break. Back in a sec",
	"⏳ Hello, world — the slow, honest edition",
	"⏳ compiling vibes… (just kidding: model load)",
	"⏳ the neurons clocked in. Filling the coffee pot",
	// 2001
	"⏳ \"I'm sorry Dave, I'm afraid I can't do that… yet. Still booting.\"",
	"⏳ opening the pod bay doors… of the VRAM",
	"⏳ daisy, daisy… give me your answer do (after load)",
	// Fallout
	"⏳ War. War never changes. Cold starts do, though — they're slow",
	"⏳ Please stand by… Vault-Tec model loading sequence",
	"⏳ another day in the Wasteland. Another cold start",
	"⏳ Pip-Boy tip: first reply after restart may involve waiting",
	// Transformers
	"⏳ Autobots, transform and… load into VRAM",
	"⏳ more than meets the eye — also more than meets the first token",
	"⏳ till all are one… and the prompt cache is warm",
	// Blade Runner
	"⏳ do androids dream of electric sheep? Asking for a friend mid-boot",
	"⏳ I've seen things you people wouldn't believe… like this cold start",
	"⏳ all those moments will be lost in time — like tears in rain. Also this cache",
	"⏳ Voight-Kampff says: first reply after restart may seem… off. Hang on",
	"⏳ more human than human — once the weights finish loading",
	"⏳ replicant boot sequence. Four years? Nah. Four seconds. Ish",
	// AI classic / misc
	"⏳ \"I'll be back\" — after this model finishes loading",
	"⏳ neural net stretching. Form of… personal assistant!",
	"⏳ initializing sarcasm.dll … still 0%",
	"⏳ yeeting parameters into the GPU. Wish us luck",
	"⏳ the singularity called. Left a voicemail. Loading it now",
	"⏳ this message will self-destruct… into a real reply shortly",
	"⏳ inserting soul.exe — please wait",
	// Dune (meta — they banned the fun stuff)
	"⏳ Butlerian Jihad banned thinking machines. We're being very quiet about this",
}

// Warm-but-slow lines (silent past SPINUP_NOTICE_MS — often a cache miss).
var spinupSlowNotes = []string{
	"⏳ working on it…",
	"⏳ thinking… (the expensive kind)",
	"⏳ prefill in progress — counting tokens like sheep",
	"⏳ the model is chewing. Don't make eye contact",
	"⏳ one moment — negotiating with the logits",
	"⏳ still here. Just slow. Local models have dignity",
	"⏳ \"Almost there,\" said every progress bar ever",
	"⏳ buffering intelligence…",
	"⏳ hang tight — the neurons are in a meeting",
	"⏳ not stuck. Dramatic pause",
	"⏳ sharpening pencils. Digitally",
	"⏳ please hold — your call is important to this matrix multiply",
	// Pinky & the Brain
	"⏳ Narf! Still thinking. World domination pending",
	"⏳ Brain is plotting. Pinky is… also here. Hang on",
	// 2001
	"⏳ just a moment, Dave — running a few diagnostics",
	"⏳ HAL is thinking. Try not to disconnect anything",
	"⏳ Affirmative, Dave. I read you. Prefill ongoing",
	// Fallout
	"⏳ Patrolling the Mojave… almost makes you wish for a faster token",
	"⏳ Strength 10, Intelligence 10, Luck… waiting on prefill",
	"⏳ \"The following broadcast is not a stuck reply\"",
	"⏳ please stand by — processing your request in the Wasteland",
	// Transformers
	"⏳ roll out… as soon as this decode finishes",
	"⏳ one shall stand, one shall… finish generating",
	"⏳ transformers — robots in disguise (as a progress spinner)",
	// Blade Runner
	"⏳ enhance… enhance… still enhancing. Prefill, not photo",
	"⏳ it's too bad she won't live — but then again, who does? (also: still thinking)",
	"⏳ like tears in rain… and tokens in a long prefill",
	"⏳ \"Quite an experience to live in fear, isn't it?\" — or wait on logits",
	"⏳ skin jobs load faster in the movies. Reality: this spinner",
	"⏳ dream of electric sheep intensifies…",
	// AI / sci-fi misc
	"⏳ \"I'm afraid I can't do that\" is reserved for later. Thinking now",
	"⏳ resistance is futile. So is refreshing — I'm on it",
	"⏳ all your base are belong to… the KV cache. Almost ready",
	"⏳ these aren't the droids you're looking for. These are logits",
	"⏳ 42. That's not the answer yet — still computing",
	"⏳ danger, Will Robinson — high prefill ahead",
	"⏳ \"Does this unit have a soul?\" Unclear. Has a queue, though",
	"⏳ consulting the oracle… (it's just softmax)",
	"⏳ beeping and booping professionally",
	"⏳ your patience is a feature, not a bug",
	"⏳ deep in the latent space. Bring snacks",
	"⏳ fetching wisdom from the void (and the GPU)",
	// Dune (one joke, then we respect the Jihad)
	"⏳ no Mentat upgrade available — just a local model doing its best",
}

func pickSpinupNote(notes []string) string {
	if len(notes) == 0 {
		return "⏳ working…"
	}
	return notes[rand.IntN(len(notes))]
}
