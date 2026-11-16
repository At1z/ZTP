import { api, addToCart, el, errorMessage, formatPrice, initLayout, stockTag, thumb, toast } from "./app.js";

initLayout("products");

const list = document.getElementById("products");

async function load() {
  try {
    const products = await api("/products");
    list.replaceChildren(...products.map(card));
  } catch (err) {
    list.replaceChildren(el("div", { class: "error-box" }, errorMessage(err)));
  }
}

function card(p) {
  const button = el("button", { class: "btn btn-small", disabled: p.available <= 0 }, "Do koszyka");
  button.addEventListener("click", async () => {
    button.disabled = true;
    try {
      await addToCart(p.id, 1);
      toast(`Dodano do koszyka: ${p.name}`, "success");
    } catch (err) {
      toast(errorMessage(err), "error");
    }
    await load(); // stock may have changed, also because of other users
  });

  return el(
    "article",
    { class: "card" },
    el(
      "a",
      { class: "card-body", href: `product.html?id=${encodeURIComponent(p.id)}` },
      thumb(p.name),
      el("h3", {}, p.name),
      el("p", {}, p.description),
    ),
    el(
      "div",
      { class: "card-footer" },
      el("div", {}, el("div", { class: "price" }, formatPrice(p.price_cents)), stockTag(p.available)),
      button,
    ),
  );
}

load();
