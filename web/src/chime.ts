let ctx: AudioContext | null = null;

// Two-note "you're up" chime via Web Audio, no asset files needed.
export function playChime() {
  try {
    ctx = ctx ?? new AudioContext();
    ctx.resume();
    const now = ctx.currentTime;
    [0, 0.18].forEach((t, i) => {
      const osc = ctx!.createOscillator();
      const gain = ctx!.createGain();
      osc.type = 'sine';
      osc.frequency.value = i === 0 ? 880 : 1174;
      gain.gain.setValueAtTime(0.0001, now + t);
      gain.gain.exponentialRampToValueAtTime(0.35, now + t + 0.04);
      gain.gain.exponentialRampToValueAtTime(0.0001, now + t + 0.6);
      osc.connect(gain).connect(ctx!.destination);
      osc.start(now + t);
      osc.stop(now + t + 0.65);
    });
  } catch {
    /* audio blocked — ignore */
  }
}