BIN := ticker

.PHONY: build run clean

build:
	go build -o $(BIN) .

run: build
	./$(BIN) ticker run --prefix tick --count 3

clean:
	rm -f $(BIN)
