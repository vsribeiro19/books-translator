export type BlockType = "heading" | "paragraph";

export interface Block {
  type: BlockType;
  level: number;
  text: string;
}

export interface Chapter {
  title: string;
  blocks: Block[];
}

export interface ExtractResult {
  pageCount: number;
  chapters: Chapter[];
}

export interface RebuildInput {
  title: string;
  chapters: Chapter[];
}
