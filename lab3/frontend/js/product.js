import { api, addToCart, el, errorMessage, formatPrice, initLayout, stockTag, thumb, toast } from "./app.js";

initLayout("products");

const root = document.getElementById("product");
const id = new URLSearchParams(location.search).get("id");

async function load() {
  try {
    render(await api(`/products/${encodeURIComponent(id)}`));
  } catch (err) {
    root.replaceChildren(el("div", { class: "error-box" }, errorMessage(err)));
  }
}

function render(p) {
  const qty = el("input", { type: "number", min: "1", max: String(Math.max(p.available, 1)), value: "1", "aria-label": "Ilość" });
  const button = el("button", { class: "btn", disabled: p.available <= 0 }, "Dodaj do koszyka");

  button.addEventListener("click", async () => {
    const quantity = Number.parseInt(qty.value, 10);
    if (!Number.isInteger(quantity) || quantity < 1) {
      toast("Podaj poprawną ilość", "error");
      return;
    }
    button.disabled = true;
    try {
      await addToCart(p.id, quantity);
      toast(`Dodano do koszyka: ${quantity} × ${p.name}`, "success");
    } catch (err) {
      toast(errorMessage(err), "error");
    }
    await load();
  });

  root.replaceChildren(
    el(
      "div",
      { class: "panel detail" },
      thumb(p.name),
      el(
        "div",
        {},
        el("h1", {}, p.name),
        stockTag(p.available),
        el("div", { class: "price" }, formatPrice(p.price_cents)),
        el("p", {}, p.description),
        el(
          "p",
          { class: "subtitle" },
          "Dodanie do koszyka blokuje produkt dla innych użytkowników na czas rezerwacji."
        ),
        el("div", { class: "buy" }, qty, button),
      ),
    ),
  );
  document.title = `${p.name} | Sklep`;
}

load();
