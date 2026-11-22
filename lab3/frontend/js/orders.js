import { api, el, errorMessage, formatPrice, formatTime, initLayout } from "./app.js";

initLayout("orders");

const root = document.getElementById("orders");

async function load() {
  try {
    const notes = await api("/notifications");
    if (notes.length === 0) {
      root.replaceChildren(
        el(
          "div",
          { class: "panel empty" },
          el("div", { class: "big" }, "📭"),
          el("p", {}, "Nie masz jeszcze zamówień."),
          el("a", { class: "btn", href: "index.html" }, "Przeglądaj produkty"),
        ),
      );
      return;
    }
    root.replaceChildren(el("div", { class: "panel" }, notes.map(note)));
  } catch (err) {
    root.replaceChildren(el("div", { class: "error-box" }, errorMessage(err)));
  }
}

function note(n) {
  const details = el("div", { hidden: true });
  const toggle = el("button", { class: "btn btn-small btn-ghost" }, "Szczegóły zamówienia");
  let loaded = false;

  toggle.addEventListener("click", async () => {
    details.hidden = !details.hidden;
    if (details.hidden || loaded) return;
    try {
      const order = await api(`/orders/${encodeURIComponent(n.order_id)}`);
      details.replaceChildren(
        el(
          "div",
          { class: "order-detail" },
          el("div", {}, el("span", {}, "Status"), el("strong", {}, order.status)),
          order.items.map((it) =>
            el("div", {}, el("span", {}, `${it.quantity} × ${it.product_name}`), el("span", {}, formatPrice(it.unit_price_cents * it.quantity))),
          ),
          el("div", {}, el("strong", {}, "Razem"), el("strong", {}, formatPrice(order.total_cents))),
        ),
      );
      loaded = true;
    } catch (err) {
      details.replaceChildren(el("div", { class: "error-box" }, errorMessage(err)));
    }
  });

  return el(
    "div",
    { class: "note" },
    el("div", { class: "note-head" }, el("strong", {}, n.message), el("time", {}, formatTime(n.created_at))),
    el("div", { style: "margin-top: 8px" }, toggle),
    details,
  );
}

load();
