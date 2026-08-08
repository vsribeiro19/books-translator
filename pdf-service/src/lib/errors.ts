export class InvalidPdfError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "InvalidPdfError";
  }
}

export class NoTextError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "NoTextError";
  }
}
