const PDFDocument = require("pdfkit");
const fs = require("fs");

const doc = new PDFDocument({ margin: 60 });
doc.pipe(fs.createWriteStream("data/sample.pdf"));

doc.fontSize(28).text("Chapter 1: Introduction");
doc.moveDown(2);
doc.fontSize(12).text(
  "This is the first paragraph of the introduction. It has some text that should be extracted properly to test the pipeline."
);
doc.moveDown(2);
doc.fontSize(12).text(
  "A second paragraph follows after a blank line, making it distinct from the previous one."
);
doc.moveDown(2);
doc.fontSize(12).text("Centered note: not a heading but short text.");
doc.addPage();
doc.fontSize(28).text("Chapter 2: Methods");
doc.moveDown(2);
doc.fontSize(12).text("Second chapter text goes here with more content.");
doc.moveDown(2);
doc.fontSize(18).text("Subsection: Sampling");
doc.moveDown(2);
doc.fontSize(12).text("Detail about sampling strategy.");
doc.end();