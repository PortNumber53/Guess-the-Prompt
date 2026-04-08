package guesscmd

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	mrand "math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"guessapi/internal/config"
	"guessapi/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var scaffoldCmd = &cobra.Command{
	Use:   "scaffold",
	Short: "Seeds the database with sample puzzles for testing",
	Run: func(cmd *cobra.Command, args []string) {
		count, _ := cmd.Flags().GetInt("count")
		clearExisting, _ := cmd.Flags().GetBool("clear")
		generateImages, _ := cmd.Flags().GetBool("images")
		objectsDir, _ := cmd.Flags().GetString("objects-dir")
		usePresets, _ := cmd.Flags().GetBool("presets")
		ollamaModel, _ := cmd.Flags().GetString("ollama-model")
		ollamaURL, _ := cmd.Flags().GetString("ollama-url")

		dbUrl := config.AppConfig.DatabaseURL
		if dbUrl == "" {
			dbUrl = "postgres://postgres:postgres@localhost:5432/guessdb?sslmode=disable"
		}

		database, err := db.Connect(context.Background(), dbUrl)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer database.Pool.Close()

		if err := ScaffoldPuzzles(context.Background(), database.Pool, count, clearExisting, generateImages, objectsDir, usePresets, ollamaModel, ollamaURL); err != nil {
			log.Fatalf("Scaffold failed: %v", err)
		}
	},
}

var cleanupOrphanImagesCmd = &cobra.Command{
	Use:   "cleanup-orphan-images",
	Short: "Remove images on disk without DB references and reset missing references",
	Run: func(cmd *cobra.Command, args []string) {
		objectsDir, _ := cmd.Flags().GetString("objects-dir")
		objectsBaseURL, _ := cmd.Flags().GetString("objects-base-url")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		placeholder, _ := cmd.Flags().GetString("placeholder")

		dbUrl := config.AppConfig.DatabaseURL
		if dbUrl == "" {
			dbUrl = "postgres://postgres:postgres@localhost:5432/guessdb?sslmode=disable"
		}

		database, err := db.Connect(context.Background(), dbUrl)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer database.Pool.Close()

		if err := CleanupOrphanImages(context.Background(), database.Pool, objectsDir, objectsBaseURL, placeholder, dryRun); err != nil {
			log.Fatalf("Cleanup orphan images failed: %v", err)
		}
	},
}

var scaffoldImagesCmd = &cobra.Command{
	Use:   "scaffold-images",
	Short: "Generate AI images for puzzles that have placeholder URLs",
	Run: func(cmd *cobra.Command, args []string) {
		objectsDir, _ := cmd.Flags().GetString("objects-dir")
		objectsBaseURL, _ := cmd.Flags().GetString("objects-base-url")
		batchSize, _ := cmd.Flags().GetInt("batch-size")

		dbUrl := config.AppConfig.DatabaseURL
		if dbUrl == "" {
			dbUrl = "postgres://postgres:postgres@localhost:5432/guessdb?sslmode=disable"
		}

		database, err := db.Connect(context.Background(), dbUrl)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer database.Pool.Close()

		if err := ScaffoldImages(context.Background(), database.Pool, objectsDir, objectsBaseURL, batchSize); err != nil {
			log.Fatalf("Scaffold images failed: %v", err)
		}
	},
}

var cleanupDuplicatesCmd = &cobra.Command{
	Use:   "cleanup-duplicates",
	Short: "Remove duplicate puzzles that still use placeholder/external images",
	Run: func(cmd *cobra.Command, args []string) {
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		dbUrl := config.AppConfig.DatabaseURL
		if dbUrl == "" {
			dbUrl = "postgres://postgres:postgres@localhost:5432/guessdb?sslmode=disable"
		}

		database, err := db.Connect(context.Background(), dbUrl)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer database.Pool.Close()

		if err := CleanupDuplicatePuzzles(context.Background(), database.Pool, dryRun); err != nil {
			log.Fatalf("Cleanup failed: %v", err)
		}
	},
}

func init() {
	scaffoldCmd.Flags().IntP("count", "n", 20, "Number of puzzles to create")
	scaffoldCmd.Flags().BoolP("clear", "c", false, "Clear existing puzzles before seeding")
	scaffoldCmd.Flags().BoolP("images", "i", false, "Generate AI images for puzzles (requires mflux)")
	scaffoldCmd.Flags().String("objects-dir", "./objects", "Directory to save generated images")
	scaffoldCmd.Flags().Bool("presets", false, "Use preset templates instead of Ollama for prompts")
	scaffoldCmd.Flags().String("ollama-model", "gemma4", "Ollama model to use for prompt generation")
	scaffoldCmd.Flags().String("ollama-url", "http://localhost:11434", "Ollama base URL")

	scaffoldImagesCmd.Flags().String("objects-dir", "./objects", "Directory to save generated images")
	scaffoldImagesCmd.Flags().String("objects-base-url", "/objects/", "Base URL for generated images")
	scaffoldImagesCmd.Flags().IntP("batch-size", "b", 5, "Number of images to generate")

	cleanupDuplicatesCmd.Flags().Bool("dry-run", false, "Show which puzzles would be deleted without mutating data")
	cleanupOrphanImagesCmd.Flags().String("objects-dir", "./objects", "Directory containing generated images")
	cleanupOrphanImagesCmd.Flags().String("objects-base-url", "/objects/", "Base URL prefix stored in DB for generated images")
	cleanupOrphanImagesCmd.Flags().String("placeholder", "/ai-generated-image.png", "Image URL to set when the referenced file is missing")
	cleanupOrphanImagesCmd.Flags().Bool("dry-run", true, "Preview changes without deleting files or updating DB")

	mrand.Seed(time.Now().UnixNano())
}

type PuzzleTemplate struct {
	Prompt string
	Prize  int
	Image  string
	Status string
	Tags   []string
}

func ScaffoldPuzzles(ctx context.Context, pool *pgxpool.Pool, count int, clearExisting bool, generateImages bool, objectsDir string, usePresets bool, ollamaModel, ollamaURL string) error {
	log.Printf("Scaffolding %d puzzles...", count)

	if generateImages && !isMfluxAvailable() {
		log.Println("WARNING: mflux-generate-z-image-turbo not found. Install with: uv tool install mflux")
		log.Println("Continuing with placeholder images...")
		generateImages = false
	}

	if clearExisting {
		log.Println("Clearing existing puzzles...")
		_, err := pool.Exec(ctx, "DELETE FROM puzzle_guesses")
		if err != nil {
			return err
		}
		_, err = pool.Exec(ctx, "DELETE FROM puzzles")
		if err != nil {
			return err
		}
		log.Println("Existing puzzles cleared")
	}

	if generateImages {
		if err := os.MkdirAll(objectsDir, 0755); err != nil {
			return fmt.Errorf("create objects directory: %w", err)
		}
	}

	var puzzles []PuzzleTemplate
	var err error
	if usePresets {
		puzzles = generatePuzzleTemplates(count)
	} else {
		puzzles, err = generateOllamaPuzzleTemplates(ctx, count, ollamaModel, ollamaURL)
		if err != nil {
			log.Printf("WARNING: Ollama prompt generation failed, falling back to presets: %v", err)
			puzzles = generatePuzzleTemplates(count)
		}
	}
	created := 0

	for i, p := range puzzles {
		desc := p.Prompt
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}
		log.Printf("  [%d/%d] Creating puzzle: %s", i+1, count, desc)

		imageURL := p.Image
		if generateImages {
			log.Printf("    Generating image for puzzle...")
			generatedURL, err := generateAndSaveImage(p.Prompt, objectsDir, "")
			if err != nil {
				log.Printf("    WARNING: Failed to generate image: %v", err)
			} else {
				imageURL = generatedURL
				log.Printf("    Image saved: %s", imageURL)
			}
		}

		_, err := pool.Exec(ctx, `
			INSERT INTO puzzles (prompt, prize_pool, image_url, status, tags)
			VALUES ($1, $2, $3, $4, $5)
		`, p.Prompt, p.Prize, imageURL, p.Status, p.Tags)

		if err != nil {
			log.Printf("    WARNING: Failed to create puzzle: %v", err)
			continue
		}
		created++
	}

	log.Printf("Scaffold complete: %d/%d puzzles created", created, count)
	return nil
}

func ScaffoldImages(ctx context.Context, pool *pgxpool.Pool, objectsDir, objectsBaseURL string, batchSize int) error {
	if !isMfluxAvailable() {
		return fmt.Errorf("mflux-generate-z-image-turbo not found — install with: uv tool install mflux")
	}

	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		return fmt.Errorf("create objects directory: %w", err)
	}

	// Find puzzles with external/placeholder images
	rows, err := pool.Query(ctx, `
		SELECT id, prompt, image_url FROM puzzles
		WHERE image_url LIKE 'https://images.unsplash.com/%'
		OR image_url = '/ai-generated-image.png'
		ORDER BY id
		LIMIT $1
	`, batchSize)
	if err != nil {
		return fmt.Errorf("query puzzles: %w", err)
	}
	defer rows.Close()

	type puzzle struct {
		id       int
		prompt   string
		imageURL string
	}

	var puzzles []puzzle
	for rows.Next() {
		var p puzzle
		if err := rows.Scan(&p.id, &p.prompt, &p.imageURL); err != nil {
			return fmt.Errorf("scan puzzle: %w", err)
		}
		puzzles = append(puzzles, p)
	}

	if len(puzzles) == 0 {
		log.Println("No puzzles need image generation")
		return nil
	}

	log.Printf("Found %d puzzles to generate images for:", len(puzzles))
	for _, p := range puzzles {
		log.Printf("  - Puzzle #%d: %s", p.id, p.prompt[:min(50, len(p.prompt))])
	}

	generated := 0
	for _, p := range puzzles {
		log.Printf("Generating image for puzzle #%d...", p.id)

		imgURL, err := generateAndSaveImage(p.prompt, objectsDir, objectsBaseURL)
		if err != nil {
			log.Printf("  WARNING: Failed to generate image: %v", err)
			continue
		}

		_, err = pool.Exec(ctx, `
			UPDATE puzzles SET image_url = $1 WHERE id = $2
		`, imgURL, p.id)
		if err != nil {
			log.Printf("  WARNING: Failed to update puzzle image: %v", err)
			continue
		}

		log.Printf("  Updated puzzle #%d with image: %s", p.id, imgURL)
		generated++
	}

	log.Printf("Scaffold-images complete: %d/%d images generated", generated, len(puzzles))
	return nil
}

func isMfluxAvailable() bool {
	_, err := exec.LookPath("mflux-generate-z-image-turbo")
	return err == nil
}

func generateAndSaveImage(prompt, objectsDir, objectsBaseURL string) (string, error) {
	// Generate random filename
	randBytes := make([]byte, 16)
	if _, err := crand.Read(randBytes); err != nil {
		return "", fmt.Errorf("random filename: %w", err)
	}
	filename := hex.EncodeToString(randBytes) + ".png"
	destPath := filepath.Join(objectsDir, filename)

	// Truncate prompt if too long (mflux may have limits)
	truncatedPrompt := prompt
	if len(truncatedPrompt) > 200 {
		truncatedPrompt = truncatedPrompt[:200]
	}

	cmd := exec.Command(
		"mflux-generate-z-image-turbo",
		"--prompt", truncatedPrompt,
		"--width", "1024",
		"--height", "1024",
		"--steps", "9",
		"-q", "8",
		"--output", destPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("mflux: %w", err)
	}

	if _, err := os.Stat(destPath); err != nil {
		return "", fmt.Errorf("image file not created: %w", err)
	}

	if objectsBaseURL != "" {
		return strings.TrimRight(objectsBaseURL, "/") + "/" + filename, nil
	}
	return "/objects/" + filename, nil
}

func generatePuzzleTemplates(count int) []PuzzleTemplate {
	var allTemplates = []PuzzleTemplate{
		{Prompt: "A highly detailed, neon-lit cyberpunk cityscape during golden hour", Prize: 0, Image: "/ai-generated-image.png", Status: "active", Tags: []string{"cyberpunk", "city", "neon"}},
		{Prompt: "A surreal floating island with waterfalls cascading into starry space", Prize: 0, Image: "https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"surreal", "space"}},
		{Prompt: "A cozy cabin in a snowy forest illuminated by warm lantern light", Prize: 0, Image: "https://images.unsplash.com/photo-1542401886-65d6c61de152?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"winter", "nature"}},
		{Prompt: "Steampunk mechanical dragon breathing blue fire in an industrial cavern", Prize: 0, Image: "https://images.unsplash.com/photo-1534447677768-be436bb09401?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"steampunk", "dragon"}},
		{Prompt: "a gigantic ancient tree glowing with ethereal green magic in a dense enchanted forest", Prize: 0, Image: "https://images.unsplash.com/photo-1518531933037-91b2f5f229cc?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"nature", "magic", "forest"}},
		{Prompt: "astronaut exploring a crystalline alien planet under a purple nebula sky", Prize: 0, Image: "https://images.unsplash.com/photo-1451187580459-43490279c0fa?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"space", "sci-fi"}},
		{Prompt: "abstract geometric shapes reflecting light inside a crystal cave", Prize: 0, Image: "https://images.unsplash.com/photo-1522030299830-16b8d3d049fe?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"abstract", "crystals"}},
		{Prompt: "bioluminescent jellyfish swimming through an underwater cave system", Prize: 0, Image: "https://images.unsplash.com/photo-1551244072-5d12893278ab?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"ocean", "nature", "glow"}},
		{Prompt: "vintage dieselpunk airship battle above cloud city at sunset", Prize: 0, Image: "https://images.unsplash.com/photo-1477696958039-8db95a362e3e?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"steampunk", "clouds", "battle"}},
		{Prompt: "medieval castle perched on a cliff overlooking stormy seas", Prize: 0, Image: "https://images.unsplash.com/photo-1534447677768-be436bb09401?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"castle", "storm", "medieval"}},
		{Prompt: "futuristic city built inside a massive Martian canyon", Prize: 0, Image: "https://images.unsplash.com/photo-1541873676-a18131494184?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"mars", "sci-fi", "city"}},
		{Prompt: "enchanted library with floating books and spiral staircases", Prize: 0, Image: "https://images.unsplash.com/photo-1507842217343-583bb7270b66?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"magic", "books", "fantasy"}},
		{Prompt: "giant mechanical whale swimming through clouds at dawn", Prize: 0, Image: "https://images.unsplash.com/photo-1516683037151-9a6e9a4f0f33?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"steampunk", "whale", "clouds"}},
		{Prompt: "ancient Egyptian tomb with holographic star maps on walls", Prize: 0, Image: "https://images.unsplash.com/photo-1566127444979-b3d2b654e3d7?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"egypt", "ancient", "stars"}},
		{Prompt: "Japanese zen garden with raked sand patterns and cherry blossoms", Prize: 0, Image: "https://images.unsplash.com/photo-1570459027562-4bb9f31aa7d1?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"japan", "zen", "garden"}},
		{Prompt: "underwater coral reef city inhabited by merpeople", Prize: 0, Image: "https://images.unsplash.com/photo-1544551763-46a013bb70d5?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"ocean", "mermaids", "city"}},
		{Prompt: "post-apocalyptic highway overgrown with mutant vegetation", Prize: 0, Image: "https://images.unsplash.com/photo-1534260164206-2a3dcb38f08d?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"apocalypse", "nature", "road"}},
		{Prompt: "clockwork owl perched on a brass gear tree", Prize: 0, Image: "https://images.unsplash.com/photo-1518495973542-4542c0a4fd8a?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"steampunk", "owl", "mechanical"}},
		{Prompt: "northern lights dancing over an icy polar research station", Prize: 0, Image: "https://images.unsplash.com/photo-1531366936337-7c912a4589a7?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"arctic", "aurora", "ice"}},
		{Prompt: "floating marketplace with hot air balloons and silk banners", Prize: 0, Image: "https://images.unsplash.com/photo-1507608616759-54f48f0af0ee?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"fantasy", "market", "balloons"}},
		{Prompt: "a mystical phoenix rising from volcanic ashes with flames of gold and crimson", Prize: 0, Image: "https://images.unsplash.com/photo-1518709268805-4e9042af9f23?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"phoenix", "fire", "fantasy"}},
		{Prompt: "an abandoned amusement park reclaimed by nature with vines on roller coasters", Prize: 0, Image: "https://images.unsplash.com/photo-1516131206008-dd041a9764fd?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"abandoned", "nature", "ruins"}},
		{Prompt: "a crystal palace floating in the clouds with rainbow bridges", Prize: 0, Image: "https://images.unsplash.com/photo-1518173946687-a4c036bc3c95?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"palace", "clouds", "crystal"}},
		{Prompt: "cybernetic samurai warrior standing in a digital dojo with holographic katanas", Prize: 0, Image: "https://images.unsplash.com/photo-1578632292335-df3abbb0d586?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"samurai", "cyberpunk", "warrior"}},
		{Prompt: "a hidden waterfall inside a glowing mushroom forest at twilight", Prize: 0, Image: "https://images.unsplash.com/photo-1441974231531-c6227db76b6e?q=80&w=600&auto=format&fit=crop", Status: "active", Tags: []string{"forest", "waterfall", "glow"}},
	}

	mrand.Shuffle(len(allTemplates), func(i, j int) {
		allTemplates[i], allTemplates[j] = allTemplates[j], allTemplates[i]
	})

	var result []PuzzleTemplate
	for i := 0; i < count; i++ {
		template := allTemplates[i%len(allTemplates)]
		result = append(result, template)
	}

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type ollamaPuzzle struct {
	Prompt string   `json:"prompt"`
	Tags   []string `json:"tags"`
	Status string   `json:"status"`
	Prize  int      `json:"prize"`
}

func generateOllamaPuzzleTemplates(ctx context.Context, count int, model, baseURL string) ([]PuzzleTemplate, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/generate"
	prompt := fmt.Sprintf(`Return exactly %d unique puzzle descriptions as a JSON array.
Each object must have:
- "prompt": imaginative description under 160 characters.
- "tags": array of 3 short lowercase keywords.
Respond with JSON only, no extra text.`, count)

	payload := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama responded with status %s", resp.Status)
	}

	var ollamaResp struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	content := strings.TrimSpace(ollamaResp.Response)
	if content == "" {
		return nil, fmt.Errorf("ollama returned empty response")
	}

	// Try to extract JSON from markdown code blocks if present
	jsonContent := extractJSONFromResponse(content)

	var raw []ollamaPuzzle
	if err := json.Unmarshal([]byte(jsonContent), &raw); err != nil {
		// Log the problematic content for debugging
		log.Printf("DEBUG: Ollama raw response: %s", content[:min(500, len(content))])
		return nil, fmt.Errorf("parse ollama json: %w (response was: %s...)", err, jsonContent[:min(200, len(jsonContent))])
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("ollama returned empty puzzle array")
	}

	var templates []PuzzleTemplate
	for _, item := range raw {
		if item.Prompt == "" {
			continue
		}
		tags := item.Tags
		if len(tags) == 0 {
			tags = fallbackTagsFromPrompt(item.Prompt)
		}
		status := "active"
		prize := 0
		templates = append(templates, PuzzleTemplate{
			Prompt: item.Prompt,
			Prize:  prize,
			Image:  "/ai-generated-image.png",
			Status: status,
			Tags:   tags,
		})
		if len(templates) >= count {
			break
		}
	}

	return templates, nil
}

func fallbackTagsFromPrompt(prompt string) []string {
	clean := strings.ToLower(prompt)
	clean = strings.ReplaceAll(clean, ",", " ")
	fields := strings.Fields(clean)
	tagSet := make([]string, 0, 3)
	unique := map[string]struct{}{}
	for _, word := range fields {
		word = strings.Trim(word, "!?.")
		if len(word) < 3 {
			continue
		}
		if _, ok := unique[word]; ok {
			continue
		}
		unique[word] = struct{}{}
		tagSet = append(tagSet, word)
		if len(tagSet) == 3 {
			break
		}
	}
	for len(tagSet) < 3 {
		tagSet = append(tagSet, fmt.Sprintf("tag%d", len(tagSet)+1))
	}
	return tagSet
}

// extractJSONFromResponse attempts to extract JSON from markdown code blocks or raw response
func extractJSONFromResponse(content string) string {
	content = strings.TrimSpace(content)

	// Check for markdown code blocks
	if strings.HasPrefix(content, "```") {
		// Extract content between code block markers
		lines := strings.Split(content, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```json") || strings.HasPrefix(line, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		if len(jsonLines) > 0 {
			return strings.Join(jsonLines, "\n")
		}
	}

	// Try to find JSON array start/end
	startIdx := strings.Index(content, "[")
	if startIdx >= 0 {
		endIdx := strings.LastIndex(content, "]")
		if endIdx > startIdx {
			return content[startIdx : endIdx+1]
		}
	}

	// Return as-is if no special handling applied
	return content
}

func CleanupOrphanImages(ctx context.Context, pool *pgxpool.Pool, objectsDir, objectsBaseURL, placeholder string, dryRun bool) error {
	entries, err := os.ReadDir(objectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Objects directory %s does not exist, nothing to clean", objectsDir)
			return nil
		}
		return fmt.Errorf("read objects dir: %w", err)
	}

	fileSet := make(map[string]os.DirEntry)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileSet[entry.Name()] = entry
	}

	rows, err := pool.Query(ctx, `
		SELECT id, image_url FROM puzzles
		WHERE image_url LIKE $1 || '%'
	`, objectsBaseURL)
	if err != nil {
		return fmt.Errorf("query puzzle images: %w", err)
	}
	defer rows.Close()

	var missingIDs []int32
	usedFiles := make(map[string]struct{})

	for rows.Next() {
		var id int32
		var imageURL string
		if err := rows.Scan(&id, &imageURL); err != nil {
			return fmt.Errorf("scan puzzle image: %w", err)
		}

		filename := strings.TrimPrefix(imageURL, objectsBaseURL)
		filename = strings.TrimLeft(filename, "/")
		if filename == "" {
			continue
		}
		usedFiles[filename] = struct{}{}
		if _, exists := fileSet[filename]; !exists {
			log.Printf("Puzzle #%d references missing file %s", id, filename)
			missingIDs = append(missingIDs, id)
		}
	}

	var orphanFiles []string
	for name := range fileSet {
		if _, ok := usedFiles[name]; !ok {
			log.Printf("Image file %s is not referenced by any puzzle", name)
			orphanFiles = append(orphanFiles, name)
		}
	}

	if dryRun {
		log.Printf("Dry run: %d puzzles missing files, %d orphan files on disk", len(missingIDs), len(orphanFiles))
		return nil
	}

	if len(missingIDs) > 0 {
		_, err = pool.Exec(ctx, `
			UPDATE puzzles SET image_url = $1 WHERE id = ANY($2)
		`, placeholder, missingIDs)
		if err != nil {
			return fmt.Errorf("reset missing image urls: %w", err)
		}
		log.Printf("Reset %d puzzle image references to placeholder", len(missingIDs))
	}

	deleted := 0
	for _, name := range orphanFiles {
		path := filepath.Join(objectsDir, name)
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.Printf("WARNING: failed to remove %s: %v", path, err)
			continue
		}
		deleted++
	}

	log.Printf("Cleanup-orphan-images complete: %d files deleted, %d references reset", deleted, len(missingIDs))
	return nil
}

func CleanupDuplicatePuzzles(ctx context.Context, pool *pgxpool.Pool, dryRun bool) error {
	rows, err := pool.Query(ctx, `
		SELECT id, prompt, COALESCE(image_url, '')
		FROM puzzles
		ORDER BY prompt, id
	`)
	if err != nil {
		return fmt.Errorf("query puzzles: %w", err)
	}
	defer rows.Close()

	type record struct {
		id       int
		prompt   string
		imageURL string
	}

	groups := make(map[string][]record)
	for rows.Next() {
		var rec record
		if err := rows.Scan(&rec.id, &rec.prompt, &rec.imageURL); err != nil {
			return fmt.Errorf("scan puzzle: %w", err)
		}
		groups[rec.prompt] = append(groups[rec.prompt], rec)
	}

	var deleteIDs []int32
	for prompt, recs := range groups {
		if len(recs) <= 1 {
			continue
		}

		keepIdx := 0
		for i, rec := range recs {
			if strings.HasPrefix(rec.imageURL, "/objects/") {
				keepIdx = i
				break
			}
		}

		for i, rec := range recs {
			if i == keepIdx {
				continue
			}
			if strings.HasPrefix(rec.imageURL, "/objects/") {
				continue
			}
			log.Printf("Duplicate prompt '%s' -> removing puzzle #%d (no generated image)", prompt, rec.id)
			deleteIDs = append(deleteIDs, int32(rec.id))
		}
	}

	if len(deleteIDs) == 0 {
		log.Println("No duplicate puzzles without generated images found")
		return nil
	}

	if dryRun {
		log.Printf("Dry run: %d puzzles would be deleted", len(deleteIDs))
		return nil
	}

	_, err = pool.Exec(ctx, `DELETE FROM puzzle_guesses WHERE puzzle_id = ANY($1)`, deleteIDs)
	if err != nil {
		return fmt.Errorf("delete puzzle guesses: %w", err)
	}

	cmdTag, err := pool.Exec(ctx, `DELETE FROM puzzles WHERE id = ANY($1)`, deleteIDs)
	if err != nil {
		return fmt.Errorf("delete puzzles: %w", err)
	}

	log.Printf("Cleanup complete: %d puzzles removed", cmdTag.RowsAffected())
	return nil
}
