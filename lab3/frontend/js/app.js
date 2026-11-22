// Shared helpers for all pages: user identity, API calls, layout, formatting.

const USER_KEY = "userId";

// The user is just a UUID kept in the browser. The backend reads it from the
// X-User-ID header; there is no login.
function newUuid() {
  if (crypto.randomUUID) return crypto.randomUUID();
  const b = crypto.getRandomValues(new Uint8Array(16));
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function getUserId() {
  let id = null;
  try { id = localStorage.getItem(USER_KEY); } catch { /* storage unavailable */ }
  if (!id || !UUID_RE.test(id)) {
    id = newUuid();
    setUserId(id);
  }
  return id;
}

function setUserId(id) {
  try { localStorage.setItem(USER_KEY, id); } catch { /* keep the id for this page only */ }
}

// ---- API ----

export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

async function request(path, { method = "GET", body } = {}) {
  const res = await fetch("/api" + path, {
    method,
    headers: {
      "X-User-ID": getUserId(),
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* not JSON */ }
  if (!res.ok) throw new ApiError(res.status, data?.error || res.statusText);
  return { data, res };
}

export async function api(path, options) {
  return (await request(path, options)).data;
}

// Returns the cart (or null when the user has none) and the difference between
// the server clock and this browser's clock, so that the countdown follows the
// server's expiry time.
export async function fetchCart() {
  try {
    const { data, res } = await request("/cart");
    const serverNow = Date.parse(res.headers.get("Date"));
    return { cart: data, skewMs: Number.isNaN(serverNow) ? 0 : serverNow - Date.now() };
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return { cart: null, skewMs: 0 };
    throw err;
  }
}

// POST /cart is idempotent: it returns the open cart or creates one.
export async function addToCart(productId, quantity) {
  await api("/cart", { method: "POST" });
  const cart = await api("/cart/items", { method: "POST", body: { product_id: productId, quantity } });
  await refreshCartCount(cart);
  return cart;
}

const MESSAGES = {
  "insufficient stock": "Brak wolnych sztuk. Produkt jest zarezerwowany w czyimś koszyku albo wyprzedany.",
  "cart is empty": "Koszyk jest pusty.",
  "no open cart": "Nie masz jeszcze koszyka.",
  "item not in cart": "Tego produktu nie ma w koszyku.",
  "product not found": "Nie znaleziono produktu.",
};

export function errorMessage(err) {
  if (!(err instanceof ApiError)) return "Nie udało się połączyć z serwerem.";
  if (err.message.startsWith("reservation expired")) {
    return "Rezerwacja wygasła. Usuń produkt z koszyka i dodaj go ponownie.";
  }
  if (err.status === 502) return "Usługa jest chwilowo niedostępna. Spróbuj ponownie za chwilę.";
  return MESSAGES[err.message] || err.message;
}

// ---- formatting and DOM ----

export function formatPrice(cents) {
  return (cents / 100).toLocaleString("pl-PL", { style: "currency", currency: "PLN" });
}

export function formatTime(iso) {
  return new Date(iso).toLocaleString("pl-PL", { dateStyle: "medium", timeStyle: "short" });
}

// el("div", {class: "x", onclick: fn}, "text", childNode) builds DOM without
// innerHTML, so product names and messages can never inject markup.
export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (value == null || value === false) continue;
    if (key.startsWith("on")) node.addEventListener(key.slice(2), value);
    else node.setAttribute(key, value === true ? "" : value);
  }
  for (const child of children.flat()) {
    if (child == null || child === false) continue;
    node.append(child instanceof Node ? child : document.createTextNode(child));
  }
  return node;
}

const COLORS = ["#6366f1", "#0ea5e9", "#10b981", "#f59e0b", "#ef4444", "#ec4899", "#8b5cf6", "#14b8a6"];

// A coloured tile with the first letters of the name stands in for a photo.
export function thumb(name) {
  let hash = 0;
  for (const ch of name) hash = (hash * 31 + ch.codePointAt(0)) >>> 0;
  const a = COLORS[hash % COLORS.length];
  const b = COLORS[(hash >>> 3) % COLORS.length];
  const node = el("div", { class: "thumb", "aria-hidden": "true" }, name.slice(0, 2).toUpperCase());
  node.style.background = `linear-gradient(135deg, ${a}, ${b})`;
  return node;
}

export function stockTag(available) {
  if (available <= 0) return el("span", { class: "tag tag-out" }, "Brak");
  if (available <= 3) return el("span", { class: "tag tag-low" }, `Zostało: ${available}`);
  return el("span", { class: "tag tag-ok" }, `Dostępne: ${available}`);
}

export function toast(message, type = "info") {
  let box = document.getElementById("toasts");
  if (!box) {
    box = el("div", { id: "toasts", "aria-live": "polite" });
    document.body.append(box);
  }
  const node = el("div", { class: `toast ${type}` }, message);
  box.append(node);
  setTimeout(() => node.remove(), 4500);
}

// ---- layout ----

function cartQuantity(cart) {
  return cart ? cart.items.reduce((sum, it) => sum + it.quantity, 0) : 0;
}

export async function refreshCartCount(knownCart) {
  const badge = document.getElementById("cart-count");
  if (!badge) return;
  let cart = knownCart;
  if (cart === undefined) {
    try { cart = (await fetchCart()).cart; } catch { cart = null; }
  }
  const n = cartQuantity(cart);
  badge.textContent = n;
  badge.hidden = n === 0;
}

function userDialog() {
  const input = el("input", { type: "text", id: "user-id-input", value: getUserId(), spellcheck: "false" });
  const dialog = el(
    "dialog",
    {},
    el("h2", {}, "Twój identyfikator"),
    el("p", {}, "Zamiast logowania jesteś rozpoznawany po tym UUID. Wklej inny, aby wejść na konto innego użytkownika (np. do testu blokady produktu), albo skopiuj swój."),
    input,
    el(
      "div",
      { class: "dialog-actions" },
      el("button", { class: "btn btn-ghost", onclick: async () => {
        try { await navigator.clipboard.writeText(input.value); toast("Skopiowano", "success"); }
        catch { input.select(); }
      } }, "Kopiuj"),
      el("button", { class: "btn btn-ghost", onclick: () => dialog.close() }, "Anuluj"),
      el("button", { class: "btn", onclick: () => {
        const value = input.value.trim().toLowerCase();
        if (!UUID_RE.test(value)) { toast("To nie jest poprawny UUID", "error"); return; }
        setUserId(value);
        location.reload();
      } }, "Zapisz"),
    ),
  );
  return dialog;
}

export function initLayout(active) {
  const dialog = userDialog();
  const link = (key, href, label, extra) =>
    el("a", { href, class: key === active ? "active" : "" }, label, extra);

  const header = el(
    "header",
    { class: "site-header" },
    el(
      "div",
      { class: "container" },
      el("a", { class: "brand", href: "index.html" }, "Sklep", el("span", {}, ".")),
      el(
        "nav",
        { class: "nav" },
        link("products", "index.html", "Produkty"),
        link("cart", "cart.html", "Koszyk", el("span", { class: "badge-count", id: "cart-count", hidden: true }, "0")),
        link("orders", "orders.html", "Zamówienia"),
      ),
      el(
        "div",
        { class: "user-chip" },
        el("span", { class: "label" }, "Użytkownik"),
        el("code", {}, getUserId().slice(0, 8)),
        el("button", { class: "btn btn-small btn-ghost", onclick: () => dialog.showModal() }, "Zmień"),
      ),
    ),
  );

  document.body.prepend(header);
  document.body.append(dialog);
  refreshCartCount();
}
