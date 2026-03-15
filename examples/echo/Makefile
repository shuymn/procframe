BIN := echo

.PHONY: build run clean

build:
	go build -o $(BIN) .

run: build
	./$(BIN) echo run --message hello

clean:
	rm -f $(BIN)
