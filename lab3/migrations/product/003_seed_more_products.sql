-- Mock catalogue for demos and manual tests. The stocks are chosen on purpose:
-- 0 (sold out), 1-3 (easy to lock), and larger ones for ordinary shopping.
INSERT INTO products (name, description, price, stock) VALUES
    ('Bilet VIP', 'Wejście VIP z miejscem siedzącym. Tylko dwie sztuki.', 499.00, 2),
    ('Bluza z kapturem', 'Ciepła bluza, rozmiar M.', 149.90, 3),
    ('Plecak miejski', 'Wodoodporny plecak 20 l.', 189.00, 8),
    ('Czapka z daszkiem', 'Regulowany pasek, kolor czarny.', 39.99, 40),
    ('Notes A5', 'Notes w kratkę, 120 kartek.', 14.50, 100),
    ('Długopis żelowy', 'Niebieski, 0,5 mm.', 4.99, 200),
    ('Termos 500 ml', 'Stal nierdzewna, trzyma ciepło 12 godzin.', 79.00, 15),
    ('Słuchawki bezprzewodowe', 'Bluetooth 5.3, do 30 godzin pracy.', 249.00, 5),
    ('Naklejki zestaw', 'Zestaw 20 naklejek z logo.', 9.99, 300),
    ('Plakat koncertowy', 'Limitowana edycja, format A2.', 59.00, 4),
    ('Kalendarz 2027', 'Kalendarz ścienny z grafikami artystów.', 34.00, 12),
    ('Bilet wczesny (wyprzedany)', 'Pula wyprzedana, do testu komunikatu o braku stanu.', 99.00, 0);
