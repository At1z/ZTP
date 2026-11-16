import { api, el, errorMessage, fetchCart, formatPrice, initLayout, refreshCartCount, thumb, toast } from "./app.js";

initLayout("cart");

const root = document.getElementById("cart");
let timer = null;

async function load() {
  clearInterval(timer);
  try {
    const { cart, skewMs } = await fetchCart();
    render(cart, skewMs);
    refreshCartCount(cart);
  } catch (err) {
    root.replaceChildren(el("div", { class: "error-box" }, errorMessage(err)));
  }
}

function emptyState(text) {
  return el(
    "div",
    { class: "panel empty" },
    el("div", { class: "big" }, "🛒"),
    el("p", {}, text),
    el("a", { class: "btn", href: "index.html" }, "Przeglądaj produkty"),
  );
}

function render(cart, skewMs) {
  if (!cart || cart.items.length === 0) {
    root.replaceChildren(emptyState("Twój koszyk jest pusty."));
    return;
  }

  const countdown = el("div", { class: "countdown" });
  const checkout = el("button", { class: "btn btn-block" }, "Złóż zamówienie");
  checkout.addEventListener("click", () => doCheckout(checkout));

  root.replaceChildren(
    el(
      "div",
      { class: "cart-layout" },
      el("div", { class: "panel" }, cart.items.map(line)),
      el(
        "aside",
        { class: "panel" },
        countdown,
        el("div", { class: "summary-row" }, el("span", {}, "Produkty"), el("span", {}, String(cart.items.reduce((s, i) => s + i.quantity, 0)))),
        el("div", { class: "summary-row total" }, el("span", {}, "Razem"), el("span", {}, formatPrice(cart.total_cents))),
        checkout,
      ),
    ),
  );

  startCountdown(countdown, Date.parse(cart.expires_at), skewMs);
}

function line(item) {
  const remove = el("button", { class: "btn btn-small btn-danger" }, "Usuń");
  remove.addEventListener("click", async () => {
    remove.disabled = true;
    try {
      await api(`/cart/items/${encodeURIComponent(item.product_id)}`, { method: "DELETE" });
      toast("Usunięto z koszyka. Produkt został zwolniony.", "success");
    } catch (err) {
      toast(errorMessage(err), "error");
    }
    load();
  });

  return el(
    "div",
    { class: "line" },
    thumb(item.product_name),
    el(
      "div",
      { class: "line-info" },
      el("a", { href: `product.html?id=${encodeURIComponent(item.product_id)}` }, el("strong", {}, item.product_name)),
      el("span", {}, `${item.quantity} × ${formatPrice(item.unit_price_cents)}`),
    ),
    el("div", { class: "line-total" }, formatPrice(item.line_total_cents)),
    remove,
  );
}

// The countdown only informs. The server decides when the cart and its
// reservations really expire.
function startCountdown(node, expiresAtMs, skewMs) {
  const tick = () => {
    const left = Math.round((expiresAtMs - (Date.now() + skewMs)) / 1000);
    if (left <= 0) {
      clearInterval(timer);
      node.className = "countdown over";
      node.replaceChildren("Czas rezerwacji minął", el("small", {}, "Produkty zostały zwolnione."));
      setTimeout(load, 1500);
      return;
    }
    const mm = String(Math.floor(left / 60)).padStart(2, "0");
    const ss = String(left % 60).padStart(2, "0");
    node.className = left <= 60 ? "countdown low" : "countdown";
    node.replaceChildren(`Rezerwacja wygasa za ${mm}:${ss}`, el("small", {}, "Potem produkty wrócą do sprzedaży."));
  };
  tick();
  timer = setInterval(tick, 1000);
}

async function doCheckout(button) {
  button.disabled = true;
  try {
    const order = await api("/cart/checkout", { method: "POST" });
    clearInterval(timer);
    refreshCartCount(null);
    root.replaceChildren(
      el(
        "div",
        { class: "panel success" },
        el("div", { class: "big" }, "✅"),
        el("h1", {}, "Zamówienie przyjęte"),
        el("p", { class: "subtitle" }, `Kwota: ${formatPrice(order.total_cents)}. Rozpoczęliśmy realizację zamówienia.`),
        el("a", { class: "btn", href: "orders.html" }, "Zobacz powiadomienia"),
      ),
    );
  } catch (err) {
    toast(errorMessage(err), "error");
    button.disabled = false;
  }
}

load();
