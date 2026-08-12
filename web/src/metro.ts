/**
 * Поиск станции метро в чекауте: устойчив к опечаткам, регистру, ё/е и дефисам.
 * Логика повторяет серверный нечёткий поиск по каталогу (repository/search.go):
 * подстрока → общий корень → расстояние Левенштейна.
 * Сам список станций живёт в коде бэкенда (model.MetroStations)
 * и приезжает по GET /api/metro-stations — один источник правды.
 */

function normalize(s: string): string {
  return s.toLowerCase().replace(/ё/g, 'е').replace(/-/g, ' ').trim().replace(/\s+/g, ' ');
}

/** Расстояние Левенштейна на двух строках DP-таблицы. */
function levenshtein(a: string, b: string): number {
  if (!a.length) return b.length;
  if (!b.length) return a.length;
  let prev = Array.from({ length: b.length + 1 }, (_, j) => j);
  let curr = new Array<number>(b.length + 1);
  for (let i = 1; i <= a.length; i++) {
    curr[0] = i;
    for (let j = 1; j <= b.length; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      curr[j] = Math.min(prev[j] + 1, curr[j - 1] + 1, prev[j - 1] + cost);
    }
    [prev, curr] = [curr, prev];
  }
  return prev[b.length];
}

/** Слово станции похоже на введённый токен: общий корень или 1–2 опечатки. */
function fuzzyWord(word: string, token: string): boolean {
  if (word.startsWith(token)) return true;
  if (word.length >= 4 && token.length >= 4) {
    const pre = Math.min(word.length, token.length) - 2;
    if (pre >= 4 && word.slice(0, pre) === token.slice(0, pre)) return true;
  }
  const tolerance = token.length > 5 ? 2 : 1;
  return levenshtein(word, token) <= tolerance;
}

/**
 * searchStations — станции, подходящие под запрос, в порядке полезности:
 * сначала начинающиеся с запроса, затем содержащие его, затем «по опечаткам».
 * Пустой запрос отдаёт начало списка — выпадашка не бывает пустой.
 */
export function searchStations(stations: string[], query: string, limit = 8): string[] {
  const q = normalize(query);
  if (!q) return stations.slice(0, limit);

  const tokens = q.split(' ');
  const scored: { name: string; score: number }[] = [];

  for (const name of stations) {
    const norm = normalize(name);
    const words = norm.split(' ');

    let score: number | null = null;
    if (norm.startsWith(q)) score = 0;
    else if (norm.includes(q)) score = 1;
    else if (words.some((w) => w.startsWith(q))) score = 2;
    else if (tokens.every((t) => norm.includes(t) || words.some((w) => fuzzyWord(w, t)))) score = 3;

    if (score !== null) scored.push({ name, score });
  }

  // При равном совпадении короткое название вероятнее нужное («Фили» → «Фили»).
  scored.sort((a, b) => a.score - b.score || a.name.length - b.name.length);
  return scored.slice(0, limit).map((s) => s.name);
}

/** Точное совпадение с названием из списка (с поправкой на регистр, ё/е и дефис). */
export function findStation(stations: string[], value: string): string | null {
  const v = normalize(value);
  return stations.find((s) => normalize(s) === v) ?? null;
}
