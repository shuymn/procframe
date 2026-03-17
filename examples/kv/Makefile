BIN := kv

.PHONY: build run clean

build:
	go build -o $(BIN) .

run: build
	./$(BIN) kv get --key greeting

clean:
	rm -f $(BIN)
